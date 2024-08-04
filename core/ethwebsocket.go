package core

import (
	"DexExecutorgo/utils"
	"context"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"time"
)

type NodeClient struct {
	ctx           context.Context
	RpcUrl        string
	client        *ethclient.Client
	wsClient      *ethclient.Client
	WsUrl         string
	PendingTxChan chan *types.Transaction
	HeaderWsList  []HeadRsp
}

func NewDexNodeClient(ctx context.Context, rpcUrl, wsUrl string, pendingTxChan chan *types.Transaction) (*NodeClient, error) {
	var err error
	nodeCli := &NodeClient{
		ctx:           ctx,
		RpcUrl:        rpcUrl,
		WsUrl:         wsUrl,
		PendingTxChan: pendingTxChan,
	}

	if nodeCli.client, err = ethclient.Dial(nodeCli.RpcUrl); err != nil {
		return nil, err
	}
	if nodeCli.wsClient, err = ethclient.Dial(nodeCli.WsUrl); err != nil {
		return nil, err
	}
	return nodeCli, nil
}

func (n *NodeClient) SubscribeFullPendingTransactions() {
	ec := gethclient.New(n.wsClient.Client())
	subscription, err := ec.SubscribeFullPendingTransactions(n.ctx, n.PendingTxChan)
	if err != nil {
		utils.Logger.Errorf("SubscribeFullPendingTransactions error, err=%v", err)
		time.Sleep(time.Second * 5)
		n.SubscribeFullPendingTransactions()
	}

	utils.Logger.Infof("SubscribeFullPendingTransactions success, %v", subscription)
}

func (n *NodeClient) SubscribeNewHeads(ch chan *types.Header) {
	subscription, err := n.wsClient.SubscribeNewHead(n.ctx, ch)
	if err != nil {
		utils.Logger.Errorf("SubscribeNewHeads error, err=%v", err)
		time.Sleep(time.Second * 5)
		n.SubscribeNewHeads(ch)
	}

	utils.Logger.Infof("SubscribeFullPendingTransactions success, %v", subscription)
}
