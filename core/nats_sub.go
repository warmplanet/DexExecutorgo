package core

import (
	"DexExecutorgo/config"
	"github.com/warmplanet/proto/go/sdk/broker"
	"strings"
)

var (
	ReceiveSignalChan map[string]chan []byte
)

func init() {
	ReceiveSignalChan = make(map[string]chan []byte)
	for k := range config.GlobalConfig.ChainConfig {
		ReceiveSignalChan[k] = make(chan []byte)
	}
}

func StartNatsSubscribe(key string, handlers broker.SubHandlers) {
	natsSubscriber := &broker.Sub{}
	natsSubscriber.Init(config.GlobalConfig.NatsConfig, handlers)
	natsSubscriber.Subscribe(key)
	select {}
}

func SignalMsgHandler(subject string, data []byte) []byte {
	keyList := strings.Split(subject, ".")
	key := keyList[4]
	signalChan, exist := ReceiveSignalChan[key]
	if !exist {
		return nil
	}

	signalChan <- data
	return nil
}
