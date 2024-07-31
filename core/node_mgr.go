package core

import (
	"DexExecutorgo/config"
	"DexExecutorgo/utils"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/warmplanet/proto/go/order"
	"github.com/warmplanet/proto/go/ordertool_wap"
	"github.com/warmplanet/proto/go/sdk"
	"github.com/warmplanet/proto/go/sdk/broker"
	"strings"
)

var (
	NodeMgrMap map[string]*NodeMgr
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

func NewNodeMgr(rpcUrl, wsUrl string, signals []Signal, router map[string]string, addrTokens map[string]string, enemiesMap map[string]bool) *NodeMgr {
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
		cDecoder:      NewCallClientDecoder(nodeCli.client),
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
			fmt.Println(toAddress)
			if dexName, ok := n.RouterMap[toAddress]; ok {
				symbolList = n.rDecoder.DecodeToSymbol(dexName, pendingTx)
			} else if okk, _ := n.EnemiesMap[toAddress]; okk {
				fmt.Println(toAddress)
				fmt.Println(pendingTx.Params.Result.Input)
				symbolList = n.cDecoder.DecodeToSymbol(pendingTx)
			}

			rebuildOrNot := n.CheckRebuildTxOrNot(symbolList)
			if rebuildOrNot {
				n.Trade()
			}
		}
	}
}

func (n *NodeMgr) CheckRebuildTxOrNot(symbolList []string) bool {
	if len(symbolList) == 0 {
		return false
	}

	fmt.Println(n.HeaderWsList[len(n.HeaderWsList)-1])
	fmt.Println(n.Signals[len(n.Signals)-1])
	return false
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
	EthClient *ethclient.Client
}

func NewCallClientDecoder(ethClient *ethclient.Client) *CallClientDecoder {
	c := &CallClientDecoder{
		EthClient: ethClient,
	}
	return c
}

func (c *CallClientDecoder) DecodeToSymbol(pendingTx *PendingTx) []string {
	var (
		addresses []string
		result    TraceCallRes
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

	fmt.Println(result, err)
	addresses = append(addresses, result.Result.To)
	c.GetAllAddress(&result.Result.Calls, addresses)

	var symbolList []string
	for _, address := range addresses {
		fmt.Println(address)
		symbolList = append(symbolList, address)
	}
	return symbolList
}

func (c *CallClientDecoder) GetAllAddress(callRes *CallsRes, addresses []string) {
	if callRes != nil {
		addresses = append(addresses, callRes.To)
		c.GetAllAddress(callRes.Call, addresses)
	}
}

func (n *NodeMgr) Trade() {
	dexInfo := order.DexInfo{}
	ter := ordertool_wap.TradingReqInfoWAP{
		//ProjectId:    "",
		//ExchangeId:   nil,
		//ExchangeAddr: nil,
		//AccountId:    "",
		//ReqNo:        "",
		//Hedge:        "",
		//Market:       "",
		//Token:        "",
		//Quote:        "",
		//Amount:       "",
		Dex: &dexInfo,
	}

	n.Publisher.PublishMsg(DATA_ORDER_TOOL, &ter)
}
