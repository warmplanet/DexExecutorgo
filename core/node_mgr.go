package core

import (
	"DexExecutorgo/config"
	"DexExecutorgo/utils"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	common2 "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/warmplanet/proto/go/common"
	"github.com/warmplanet/proto/go/order"
	"github.com/warmplanet/proto/go/ordertool_wap"
	"github.com/warmplanet/proto/go/sdk"
	"github.com/warmplanet/proto/go/sdk/broker"
	"math"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"
)

type NodeMgr struct {
	RpcUrl        string
	WsUrl         string
	NodeClient    *NodeClient
	RouterMap     map[string]string
	EnemiesMap    map[string]bool
	PendingTxList []*types.Transaction
	PendingTxChan chan *types.Transaction
	HeaderWsList  []*types.Header
	Signals       []Signal
	SignalChan    chan []byte
	Publisher     *broker.Pub
	rDecoder      *RouterDecoder
	cDecoder      *CallClientDecoder
}

func NewNodeMgr(ctx context.Context, rpcUrl, wsUrl string, signalChan chan []byte,
	router map[string]string, addrTokens map[string]string, enemiesMap map[string]bool, pairSymbolMap map[string]string) *NodeMgr {
	pendingTxChan := make(chan *types.Transaction)
	nodeCli, err := NewDexNodeClient(ctx, rpcUrl, wsUrl, pendingTxChan)
	if err != nil {
		utils.Logger.Errorf("init dexNode obj failed, err=%v", err)
	}
	rDecoder, err := NewRouterDecoder(addrTokens)
	if err != nil {
		utils.Logger.Errorf("init router decoder failed, err=%v", err)
	}
	nodeMgr := &NodeMgr{
		RpcUrl:        rpcUrl,
		WsUrl:         wsUrl,
		NodeClient:    nodeCli,
		RouterMap:     router,
		EnemiesMap:    enemiesMap,
		PendingTxChan: pendingTxChan,
		Publisher:     NewNatsPublisher(),
		Signals:       make([]Signal, 0),
		SignalChan:    signalChan,
		rDecoder:      rDecoder,
		cDecoder:      NewCallClientDecoder(nodeCli.client, pairSymbolMap),
	}
	return nodeMgr
}

func NewNatsPublisher() *broker.Pub {
	natsPublisher := &broker.Pub{}
	natsPublisher.PubConfig = sdk.PubConfig{UniqueName: "addPrice"}
	natsPublisher.Init(config.GlobalConfig.NatsConfig)
	return natsPublisher
}

func (n *NodeMgr) Execute() {
	go n.GasPriceAnalyse()

	go n.SubGethBlockHeader()
	n.NodeClient.SubscribeFullPendingTransactions()
}

func (n *NodeMgr) SubGethBlockHeader() {
	defer func() {
		if err := recover(); err != nil {
			utils.Logger.Errorf("SubGethBlockHeader painc: %v", utils.PanicTrace(err))
			time.Sleep(time.Second)
			go n.SubGethBlockHeader()
		}
	}()

	ch := make(chan *types.Header)
	n.NodeClient.SubscribeNewHeads(ch)
	for {
		select {
		case <-n.NodeClient.ctx.Done():
			utils.Logger.Info("get ctx signal, exit to receiveMessage")
			n.NodeClient.wsClient.Close()
		case newHead, _ := <-ch:
			if newHead.Number.Int64() == 0 {
				continue
			}
			if len(n.HeaderWsList) > 200 {
				startBlockHeader, endBlockHeader := n.HeaderWsList[0], n.HeaderWsList[100]
				n.HeaderWsList = n.HeaderWsList[100:]
				utils.Logger.Infof("delete blockHeader in HeaderWsList, startTime: %v, endTime: %v", startBlockHeader.Time, endBlockHeader.Time)
			}
			n.HeaderWsList = append(n.HeaderWsList, newHead)
		}
	}
}

func (n *NodeMgr) GasPriceAnalyse() {
	defer func() {
		if err := recover(); err != nil {
			utils.Logger.Errorf("GasPriceAnalyse painc: %v", utils.PanicTrace(err))
			time.Sleep(time.Second)
			go n.GasPriceAnalyse()
		}
	}()

	for {
		select {
		case <-n.NodeClient.ctx.Done():
			utils.Logger.Info("get ctx signal, exit to receiveMessage")
			n.NodeClient.wsClient.Close()
		case tx, _ := <-n.PendingTxChan:
			var (
				alreadyOnChain bool
				pendingTx      PendingTx
			)

			txByte, _ := tx.MarshalJSON()
			err := json.Unmarshal(txByte, &pendingTx)
			if err != nil {
				utils.Logger.Errorf("json unmarshal pendingTx error, err=%v", err)
				continue
			}
			fromAddress, _ := types.Sender(types.NewLondonSigner(tx.ChainId()), tx)
			pendingTx.From = fromAddress.Hex()
			toAddress := pendingTx.To
			var symbolList []string
			if dexName, ok := n.RouterMap[toAddress]; ok {
				symbolList = n.rDecoder.DecodeToSymbol(dexName, &pendingTx)
			} else if okk, _ := n.EnemiesMap[toAddress]; okk {
				if len(n.PendingTxList) > 200 {
					n.PendingTxList = n.PendingTxList[100:]
				}
				n.PendingTxList = append(n.PendingTxList, tx)

				latestBlock := n.HeaderWsList[len(n.HeaderWsList)-1]
				symbolList, alreadyOnChain = n.cDecoder.DecodeToSymbol(&pendingTx, latestBlock)
				if !alreadyOnChain {
					utils.Logger.Infof("看到竞争对手pending，解析symbol=%v, hash=%v, pending=%v", symbolList, pendingTx.Hash, string(txByte))
				}
			}

			if len(symbolList) != 0 && len(n.Signals) != 0 {
				utils.Logger.Infof("竞争对手可能抢交易, symbol=%v", symbolList)
				if alreadyOnChain {
					n.CancelTx()
				} else {
					n.RebuildTx(symbolList, tx.GasPrice())
				}
			}
		case data, _ := <-n.SignalChan:
			var signal Signal
			err := json.Unmarshal(data, &signal)
			if err != nil {
				utils.Logger.Errorf("signal obj json unmarshal error, err=%v", err)
				continue
			}

			utils.Logger.Infof("收到一个signal, projectId=%v, registerId=%v, hedgeId=%v, signal=%v", signal.Server, signal.RegistId, signal.HedgeId, string(data))
			if len(n.Signals) > 200 {
				startSignal, endSignal := n.Signals[0], n.Signals[100]
				n.Signals = n.Signals[100:]
				utils.Logger.Infof("delete signal in signalList, startTime: %v, endTime: %v", startSignal.UpdateTime, endSignal.UpdateTime)
			}
			n.Signals = append(n.Signals, signal)

			var lastPendingTx *types.Transaction
			if len(n.PendingTxList) > 0 {
				lastPendingTx = n.PendingTxList[len(n.PendingTxList)-1]
				utils.Logger.Infof("收到signal最近的一条tx hash=%v, tx block=%v, signalTradeBlockNum=%v, time=%v", lastPendingTx.Hash(), n.GetPendingBlockNum(), signal.TradeBlockNum, time.Now())
			}
		}
	}
}

func (n *NodeMgr) GetPendingBlockNum() int64 {
	lastPendingBlock := n.HeaderWsList[len(n.HeaderWsList)-1]
	lastPendingBlockTimestamp := int64(lastPendingBlock.Time)
	timeNow := int64(time.Now().Second())
	if timeNow-lastPendingBlockTimestamp > 2 {
		return 0
	}
	return lastPendingBlock.Number.Int64() + 1
}

func (n *NodeMgr) RebuildTx(symbolList []string, txGasPrice *big.Int) bool {
	curSignal := n.GetLastSignalBySymbol(symbolList[0])
	utils.Logger.Infof("竞争对手触发，获取最近signal, signal=%v", curSignal)

	// 区块校验
	if curSignal.TradeBlockNum == n.GetPendingBlockNum() {
		newGasPrice := big.NewInt(1)
		newGasPrice.Add(newGasPrice, txGasPrice)
		baseGasPrice := big.NewInt(int64(curSignal.BaseGasPrice * math.Pow(10, 9)))
		newGasPrice.Sub(newGasPrice, baseGasPrice)
		n.Trade(curSignal, newGasPrice)
	}
	// 时间校验待添加
	return false
}

func (n *NodeMgr) CancelTx() bool {
	return false
}

func (n *NodeMgr) Trade(signal Signal, newGasPrice *big.Int) {
	var (
		side         order.TradeSide
		token, quote string
	)

	if signal.Side == "buy" {
		side = order.TradeSide_BUY
	} else {
		side = order.TradeSide_SELL
	}

	tokens := strings.Split(signal.Symbol, "_")
	token, quote = tokens[0], tokens[1]

	dexInfo := order.DexInfo{
		Signer: signal.BuildTx.From,
	}
	dexInfo.Info = &order.DexInfo_E{
		E: &order.EthInfo{
			TxType:               order.EthTransactionType_DynamicFeeTx,
			GasLimit:             uint64(signal.BuildTx.Gas),
			Nonce:                uint64(signal.BuildTx.Nonce),
			Data:                 []byte(signal.BuildTx.Data),
			MaxPriorityFeePerGas: newGasPrice.Uint64(),
			MaxFeePerGas:         uint64(signal.BuildTx.MaxFeePerGas),
			Value:                strconv.Itoa(signal.BuildTx.Value),
		},
	}

	hedgeExpect := make([]*order.OrderHedge, 0)
	for subject, depthDetails := range signal.DepthDetails {
		for _, itemDetail := range depthDetails {
			for _, item := range itemDetail.Path {
				symbol := strings.ToLower(strings.Replace(item.Symbol, "/", "_", -1))
				itemTokens := strings.Split(item.Symbol, "/")
				itemBase, itemQuote := itemTokens[0], itemTokens[1]
				amount := itemDetail.ConsumeRatio * itemDetail.AmountSum
				priceExpect := item.PriceAvg
				s := signal.SymbolsTradeSide[subject][symbol]
				var itemSide order.TradeSide
				if s == "buy" {
					itemSide = order.TradeSide_BUY
				} else {
					itemSide = order.TradeSide_SELL
				}
				orderHedge := &order.OrderHedge{
					Side:          itemSide,
					PriceExpected: priceExpect,
					Amount:        amount,
					ExpiredTimeMs: 15000 + time.Now().UnixMilli(),
					Exchange:      common.Exchange(item.Exchange),
					AccountId:     []byte("ptneurm1"),
					Market:        common.Market(item.Market),
					Type:          common.SymbolType(item.SymbolType),
					Token:         []byte(itemBase),
					Quote:         []byte(itemQuote),
				}
				hedgeExpect = append(hedgeExpect, orderHedge)
			}
		}
	}

	ter := ordertool_wap.TradingReqInfoWAP{
		ProjectId:    "avalanche_v1",
		ReqNo:        signal.RegistId,
		Hedge:        signal.HedgeId,
		TradeType:    order.TradeType_TAKER,
		TradeSide:    side,
		PriceExpect:  signal.PriceExpect,
		Token:        strings.ToUpper(token),
		Quote:        strings.ToLower(quote),
		Amount:       signal.AmountArb,
		ExchangeId:   common.Exchange_AVALANCHE_DEX,
		ExchangeAddr: signal.BuildTx.To,
		AccountId:    []byte("avalanche"),
		Market:       common.Market_SPOT,
		Type:         order.OrderSysType_TRADE_DEX,
		Contract:     common.SymbolType_SPOT_NORMAL,
		Dex:          &dexInfo,
		HedgeExpect:  hedgeExpect,
	}

	pubErr := n.Publisher.PublishMsg(DATA_ORDER_TOOL, &ter)
	if pubErr != nil {
		utils.Logger.Errorf("publish update gasPrice order error, err=%v", pubErr)
		return
	}
	terByte, _ := json.Marshal(ter)
	utils.Logger.Infof("pub ter msg, subjects=%v, msg=%v", DATA_ORDER_TOOL, string(terByte))
}

func (n *NodeMgr) GetLastSignalBySymbol(symbol string) (signal Signal) {
	for i := len(n.Signals) - 1; i >= 0; i -= 1 {
		signal = n.Signals[i]
		if signal.Symbol == symbol {
			return
		}
	}
	return
}

type Decoder interface {
	DecodeToSymbol()
}

type RouterDecoder struct {
	AddrTokens         map[string]string
	UniswapV3RouterAbi abi.ABI
	TraderjoeRouterAbi abi.ABI
}

func NewRouterDecoder(addrTokens map[string]string) (*RouterDecoder, error) {
	var err error
	routerDecoder := &RouterDecoder{
		AddrTokens: addrTokens,
	}
	if routerDecoder.UniswapV3RouterAbi, err = abi.JSON(strings.NewReader(UniV3RouterAbiData)); err != nil {
		return nil, err
	}
	if routerDecoder.TraderjoeRouterAbi, err = abi.JSON(strings.NewReader(TraderjoeRouterAbiData)); err != nil {
		return nil, err
	}
	return routerDecoder, nil
}

func (r *RouterDecoder) DecodeToSymbol(dexName string, pendingTx *PendingTx) (symbolList []string) {
	txInput := pendingTx.Input
	funcSig, err := hex.DecodeString(txInput[2:10])
	if err != nil {
		utils.Logger.Errorf("decode function signature failed, funcsig=%v, err=%v", funcSig, err)
		return
	}
	ti, _ := hex.DecodeString(txInput[10:])
	if dexName == "traderjoe" {
		method, mErr := r.TraderjoeRouterAbi.MethodById(funcSig)
		if mErr != nil {
			utils.Logger.Errorf("routerAbi get method by id failed, id=%v, err=%v", funcSig, mErr)
			return
		}
		params, upErr := method.Inputs.Unpack(ti)
		if upErr != nil {
			utils.Logger.Errorf("method inputs unpack error, params=%v, err=%v", params, mErr)
			return
		}
		pathByte, _ := json.Marshal(params[2])
		var path TraderJoePath
		pErr := json.Unmarshal(pathByte, &path)
		if pErr != nil {
			utils.Logger.Errorf("traderjoePath json unmarshal error, path=%v, err=%v", string(pathByte), pErr)
			return
		}
		tokenAddr0, tokenAddr1 := path.TokenPath[0], path.TokenPath[1]
		token0, ok0 := r.AddrTokens[tokenAddr0.String()]
		token1, ok1 := r.AddrTokens[tokenAddr1.String()]
		if ok0 && ok1 {
			symbolList = append(symbolList, fmt.Sprintf("%v_%v", token0, token1))
			symbolList = append(symbolList, fmt.Sprintf("%v_%v", token1, token0))
		}
	} else if dexName == "avaxuniswap" {
		method, mErr := r.UniswapV3RouterAbi.MethodById(funcSig)
		if mErr != nil {
			utils.Logger.Errorf("routerAbi get method by id failed, id=%v, err=%v", funcSig, mErr)
			return
		}

		params, upErr := method.Inputs.Unpack(ti)
		if upErr != nil {
			utils.Logger.Errorf("method inputs unpack error, params=%v, err=%v", params, mErr)
			return
		}
		fmt.Println(params)
		pathByte, _ := json.Marshal(params[2])
		fmt.Println(string(pathByte))
	} else {
		return
	}
	return
}

type CallClientDecoder struct {
	EthClient     *ethclient.Client
	PairSymbolMap map[string]string
}

func NewCallClientDecoder(ethClient *ethclient.Client, pairSymbolMap map[string]string) *CallClientDecoder {
	c := &CallClientDecoder{
		EthClient:     ethClient,
		PairSymbolMap: pairSymbolMap,
	}
	return c
}

func (c *CallClientDecoder) DecodeToSymbol(pendingTx *PendingTx, latestBlock *types.Header) ([]string, bool) {
	var (
		addresses []string
		result    interface{}
		paramsa   = ParamsA{
			From: pendingTx.From,
			To:   pendingTx.To,
			Data: pendingTx.Input,
		}

		paramsb = Tracer{
			Tracer: "callTracer",
			TracerConfig: TracerConfig{
				WithLog:     true,
				OnlyTopCall: false,
			},
		}
		wg             sync.WaitGroup
		alreadyOnChain = true
	)

	wg.Add(2)
	go func() {
		defer wg.Done()

		err := c.EthClient.Client().Call(&result, "debug_traceCall", paramsa, "latest", paramsb)
		if err != nil {
			utils.Logger.Errorf("call debug_traceCall error, err: %v", err)
			return
		}

		tmp, _ := json.Marshal(result)
		var resTraceCall CallsRes
		err = json.Unmarshal(tmp, &resTraceCall)
		if err != nil {
			utils.Logger.Errorf("json unmarshal CallsRes error, err: %v", err)
			return
		}
		addresses = c.GetAllAddress([]CallsRes{resTraceCall})
	}()

	go func() {
		defer wg.Done()

		_, receiptErr := c.EthClient.TransactionReceipt(context.Background(), common2.HexToHash(pendingTx.Hash))
		if receiptErr != nil {
			utils.Logger.Errorf("get pendingTransactionReceipt failed, err=%v", receiptErr)
			alreadyOnChain = false
		}
		//if receipt.BlockNumber.Int64() < latestBlock.Number.Int64() {
		//	isNotFound = true
		//}
	}()
	wg.Wait()

	var symbolList []string
	if !alreadyOnChain {
		for _, address := range addresses {
			if symbol, ok := c.PairSymbolMap[address]; ok {
				symbolList = append(symbolList, symbol)
			}
		}
	}
	return symbolList, alreadyOnChain
}

func (c *CallClientDecoder) GetAllAddress(callsRes []CallsRes) (addresses []string) {
	if len(callsRes) != 0 {
		for _, call := range callsRes {
			addresses = append(addresses, call.To)
			addresses = append(addresses, c.GetAllAddress(call.Call)...)
		}
	}
	return addresses
}
