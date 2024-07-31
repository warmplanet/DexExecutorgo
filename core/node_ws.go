package core

import (
	"DexExecutorgo/config"
	"DexExecutorgo/utils"
	"encoding/json"
	"errors"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/ethclient"
	"strings"
	"time"
)

type NodeClient struct {
	RpcUrl             string
	client             *ethclient.Client
	WsUrl              string
	UniswapV3RouterAbi abi.ABI
	TraderjoeRouterAbi abi.ABI
	PendingTxChan      chan *PendingTx
	HeaderWsList       []HeadRsp
}

func NewDexNodeClient(rpcUrl, wsUrl string, pendingTxChan chan *PendingTx, headerWsList []HeadRsp) (*NodeClient, error) {
	var err error
	nodeCli := &NodeClient{
		RpcUrl:        rpcUrl,
		WsUrl:         wsUrl,
		PendingTxChan: pendingTxChan,
		HeaderWsList:  headerWsList,
	}

	if nodeCli.client, err = ethclient.Dial(nodeCli.RpcUrl); err != nil {
		return nil, err
	}
	return nodeCli, nil
}

func (n *NodeClient) EstablishConn(subScribe wsRequest, handler func([]byte) error) error {
	var (
		readTimeout = time.Duration(5000) * time.Millisecond
		wsUrl       = n.WsUrl
	)

	wsBuilder := NewWsBuilderWithReadTimeout(readTimeout).
		ProxyUrl(config.GlobalConfig.ProxyUrl).
		WsUrl(wsUrl).
		ProtoHandleFunc(handler).
		AutoReconnect().
		//Heartbeat(Heartbeat, 29*time.Second).
		PostReconnectSuccess(
			func(wsClient *WsConn) error {
				err := wsClient.Subscribe(subScribe)
				return err
			})

	wsClient := wsBuilder.Build()
	if wsClient == nil {
		return errors.New("build websocket connection error")
	}

	err := wsClient.Subscribe(subScribe)
	if err != nil {
		return err
	}

	return nil
}

func (n *NodeClient) HeadsHandler(data []byte) error {
	var (
		headWsRsp HeadRsp
	)
	if strings.Contains(string(data), "invalid request") {
		return nil
	}
	err := json.Unmarshal(data, &headWsRsp)
	if err != nil {
		return errors.New("json unmarshal data err: " + string(data))
	}
	if headWsRsp.Method == "" {
		return nil
	}
	if len(n.HeaderWsList) > 200 {
		n.HeaderWsList = n.HeaderWsList[100:]
	}
	n.HeaderWsList = append(n.HeaderWsList, headWsRsp)
	return nil
}

func (n *NodeClient) PendingTxHandler(data []byte) error {
	var (
		pendingTx PendingTx
	)
	if strings.Contains(string(data), "invalid request") {
		return nil
	}
	err := json.Unmarshal(data, &pendingTx)
	if err != nil {
		utils.Logger.Errorf("json unmarshal data err: %v, data: %v", err, string(data))
		return errors.New("json unmarshal data err: " + err.Error())
	}
	if pendingTx.Method == "" {
		return nil
	}
	if pendingTx.Params.Error.Message != "" {
		utils.Logger.Errorf("subscribe node error, err: %v, data: %v", pendingTx.Params.Error.Message, string(data))
		return errors.New("subscribe node error, err: " + pendingTx.Params.Error.Message)
	}

	n.PendingTxChan <- &pendingTx
	return nil
}
