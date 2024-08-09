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
	ReceiveSignalChan map[string]chan Signal
)

func init() {
	ReceiveSignalsMap = make(map[string][]Signal)
	ReceiveSignalChan = make(map[string]chan Signal)
	for k := range config.GlobalConfig.ChainConfig {
		ReceiveSignalsMap[k] = []Signal{}
		ReceiveSignalChan[k] = make(chan Signal)
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
	signalChan, exist := ReceiveSignalChan[key]
	if !exist {
		return nil
	}

	var signal Signal
	err := json.Unmarshal(data, &signal)
	if err != nil {
		utils.Logger.Errorf("signal obj json unmarshal error, err=%v", err)
		return nil
	}

	utils.Logger.Infof("收到一个signal, key=%v, registerId=%v, hedgeId=%v, signal=%v", key, signal.RegistId, signal.HedgeId, string(data))
	if len(signals) > 200 {
		startSignal, endSignal := signals[0], signals[100]
		signals = signals[100:]
		utils.Logger.Infof("delete signal in signalList, startTime: %v, endTime: %v", startSignal.UpdateTime, endSignal.UpdateTime)
	}
	signals = append(signals, signal)

	signalChan <- signal
	return nil
}
