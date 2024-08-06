package core

import (
	"DexExecutorgo/config"
	"DexExecutorgo/utils"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/warmplanet/proto/go/common"
	"github.com/warmplanet/proto/go/order"
	"github.com/warmplanet/proto/go/ordertool_wap"
	"github.com/warmplanet/proto/go/sdk"
	"github.com/warmplanet/proto/go/sdk/broker"
	"strconv"
	"strings"
	"time"
)

type NodeMgr struct {
	RpcUrl        string
	WsUrl         string
	NodeClient    *NodeClient
	RouterMap     map[string]string
	EnemiesMap    map[string]bool
	PendingTxChan chan *types.Transaction
	HeaderWsList  []*types.Header
	Signals       []Signal
	Publisher     *broker.Pub
	rDecoder      *RouterDecoder
	cDecoder      *CallClientDecoder
}

func NewNodeMgr(ctx context.Context, rpcUrl, wsUrl string, signals []Signal, router map[string]string, addrTokens map[string]string, enemiesMap map[string]bool, pairSymbolMap map[string]string) *NodeMgr {
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
		Signals:       signals,
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
				n.HeaderWsList = n.HeaderWsList[100:]
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
			var pendingTx PendingTx
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
				symbolList = n.cDecoder.DecodeToSymbol(&pendingTx)
			}

			fmt.Println(symbolList)
			n.CheckRebuildTxOrNot(symbolList)
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

func (n *NodeMgr) CheckRebuildTxOrNot(symbolList []string) bool {
	if len(symbolList) == 0 || len(n.Signals) == 0 {
		return false
	}

	calBlockNum := n.GetPendingBlockNum()
	signal := n.Signals[len(n.Signals)-1]

	fmt.Println(calBlockNum, signal, signal.TradeBlockNum)
	// 时间校验待添加
	if signal.TradeBlockNum == calBlockNum {
		fmt.Println(signal.SignalBlockTime, time.Now().Second())
		//n.Trade(signal)
	}

	return false
}

func (n *NodeMgr) Trade(signal Signal) {
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
			MaxPriorityFeePerGas: uint64(signal.BuildTx.GasPrice),
			MaxFeePerGas:         uint64(signal.BuildTx.GasPrice),
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
	}
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

func (c *CallClientDecoder) DecodeToSymbol(pendingTx *PendingTx) []string {
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
	)
	err := c.EthClient.Client().Call(&result, "debug_traceCall", paramsa, "latest", paramsb)
	if err != nil {
		utils.Logger.Errorf("call debug_traceCall error, err: %v", err)
		return addresses
	}

	tmp, _ := json.Marshal(result)
	var resTraceCall CallsRes
	err = json.Unmarshal(tmp, &resTraceCall)
	if err == nil {
		addresses = c.GetAllAddress([]CallsRes{resTraceCall})
	} else {
		return addresses
	}

	var symbolList []string
	for _, address := range addresses {
		if symbol, ok := c.PairSymbolMap[address]; ok {
			symbolList = append(symbolList, symbol)
		}
	}
	return symbolList
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
