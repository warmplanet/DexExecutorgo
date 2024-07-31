package core

import (
	"DexExecutorgo/config"
	"DexExecutorgo/utils"
	"encoding/json"
	"github.com/warmplanet/proto/go/sdk/broker"
	"strings"
)

var (
	ReceiveSignalsMap map[string][]Signal
)

func init() {
	ReceiveSignalsMap = make(map[string][]Signal)
	for k := range config.GlobalConfig.ChainConfig {
		ReceiveSignalsMap[k] = []Signal{}
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
	signals, exist := ReceiveSignalsMap[key]
	if !exist {
		return nil
	}

	var signal Signal
	err := json.Unmarshal(data, &signal)
	if err != nil {
		utils.Logger.Errorf("signal obj json unmarshal error, err=%v", err)
		return nil
	}

	if len(signals) > 200 {
		signals = signals[100:]
	}
	signals = append(signals, signal)

	return nil
}
