package core

import (
	"DexExecutorgo/config"
	"DexExecutorgo/utils"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

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
		Id:      2,
		JsonRpc: 2.0,
		Method:  "eth_subscribe",
		Params:  []string{"newHeads"},
	}
	err := n.NodeClient.EstablishConn(subScribe, n.NodeClient.HeadsHandler)
	if err != nil {
		utils.Logger.Errorf("call blockHeader client error, err=%v", err)
	}
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
	//if pendingTx.Method == "" {
	//	return nil
	//}
	//if pendingTx.Params.Error.Message != "" {
	//	utils.Logger.Errorf("subscribe node error, err: %v, data: %v", pendingTx.Params.Error.Message, string(data))
	//	return errors.New("subscribe node error, err: " + pendingTx.Params.Error.Message)
	//}

	//n.PendingTxChan <- &pendingTx
	return nil
}
