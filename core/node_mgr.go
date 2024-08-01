package core

import (
	"DexExecutorgo/config"
	"DexExecutorgo/utils"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/warmplanet/proto/go/common"
	"github.com/warmplanet/proto/go/order"
	"github.com/warmplanet/proto/go/ordertool_wap"
	"github.com/warmplanet/proto/go/sdk"
	"github.com/warmplanet/proto/go/sdk/broker"
	"strings"
	"time"
)

type NodeMgr struct {
	RpcUrl        string
	WsUrl         string
	NodeClient    *NodeClient
	RouterMap     map[string]string
	EnemiesMap    map[string]bool
	PendingTxChan chan *PendingTx
	HeaderWsList  []HeadRsp
	Signals       []Signal
	Publisher     *broker.Pub
	rDecoder      *RouterDecoder
	cDecoder      *CallClientDecoder
}

func NewNodeMgr(rpcUrl, wsUrl string, signals []Signal, router map[string]string, addrTokens map[string]string, enemiesMap map[string]bool, pairSymbolMap map[string]string) *NodeMgr {
	pendingTxChan := make(chan *PendingTx)
	headerWsMap := make([]HeadRsp, 0)
	nodeCli, err := NewDexNodeClient(rpcUrl, wsUrl, pendingTxChan, headerWsMap)
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
	n.SubPendingTx()
	//n.SubBlockHeader()
	n.GasPriceAnalyse()
}

func (n *NodeMgr) SubPendingTx() {
	subScribe := wsRequest{
		Id:      1,
		JsonRpc: 2.0,
		Method:  "eth_subscribe",
		Params:  []string{"newPendingTransactions", "true"},
	}
	err := n.NodeClient.EstablishConn(subScribe, n.NodeClient.PendingTxHandler)
	if err != nil {
		utils.Logger.Errorf("call pendingTx client error, err=%v", err)
	}
}

func (n *NodeMgr) SubBlockHeader() {
	subScribe := wsRequest{
		Id:      1,
		JsonRpc: 2.0,
		Method:  "eth_subscribe",
		Params:  []string{"newHeads"},
	}
	err := n.NodeClient.EstablishConn(subScribe, n.NodeClient.HeadsHandler)
	if err != nil {
		utils.Logger.Errorf("call blockHeader client error, err=%v", err)
	}
}

func (n *NodeMgr) GasPriceAnalyse() {
	for {
		select {
		case pendingTx, _ := <-n.PendingTxChan:
			fromAddress := pendingTx.Params.Result.From
			toAddress := pendingTx.Params.Result.To
			if fromAddress == toAddress {
				utils.Logger.Infof("fromAddress is the same as toAddress, fromAddr=%v, toAddr=%v", fromAddress, toAddress)
				continue
			}

			var symbolList []string
			if dexName, ok := n.RouterMap[toAddress]; ok {
				symbolList = n.rDecoder.DecodeToSymbol(dexName, pendingTx)
			} else if okk, _ := n.EnemiesMap[toAddress]; okk {
				symbolList = n.cDecoder.DecodeToSymbol(pendingTx)
			}

			n.CheckRebuildTxOrNot(symbolList)
		}
	}
}

func (n *NodeMgr) GetPendingBlockNum() int64 {
	lastPendingBlock := n.HeaderWsList[len(n.HeaderWsList)-1]
	lastPendingBlockTimestamp := lastPendingBlock.Params.Result.Timestamp
	timeNow := time.Now().UnixMilli()
	if timeNow-lastPendingBlockTimestamp > 10*60 {
		return 0
	}
	blockNumDiff := (timeNow - lastPendingBlockTimestamp) / 2
	return lastPendingBlock.Params.Result.Number + blockNumDiff + 1
}

func (n *NodeMgr) CheckRebuildTxOrNot(symbolList []string) bool {
	if len(symbolList) == 0 {
		return false
	}

	calBlockNum := n.GetPendingBlockNum()
	signal := n.Signals[len(n.Signals)-1]
	fmt.Println(calBlockNum, signal.TradeBlockNum)

	if calBlockNum > signal.TradeBlockNum {
		return false
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
			Value:                signal.BuildTx.Value,
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
	txInput := pendingTx.Params.Result.Input
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
			From: pendingTx.Params.Result.From,
			To:   pendingTx.Params.Result.To,
			Data: pendingTx.Params.Result.Input,
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
