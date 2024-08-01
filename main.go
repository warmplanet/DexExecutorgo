package main

import (
	"DexExecutorgo/config"
	"DexExecutorgo/core"
	"DexExecutorgo/utils"
	"github.com/warmplanet/proto/go/sdk/broker"
	"strings"
)

func main() {
	utils.InitLogger(config.GlobalConfig.LogPath, config.GlobalConfig.LogLevel)

	for k, v := range config.GlobalConfig.ChainConfig {
		chainName := strings.Split(k, "_")[0]
		signalKey := strings.Replace(core.NATS_SIGNAL_SUBJECT, "{{Chain}}", chainName, -1)
		signalKey = strings.Replace(signalKey, "{{ProjectId}}", k, -1)
		go core.StartNatsSubscribe(signalKey, broker.SubHandlers{Data: core.SignalMsgHandler})

		signals := core.ReceiveSignalsMap[k]
		newRouter := make(map[string]string)
		for kk, vv := range v.Router {
			newRouter[vv] = kk
		}

		addrTokens := make(map[string]string)
		for name, token := range v.Tokens {
			addrTokens[token.Address] = name
		}

		enemiesMap := make(map[string]bool)
		for _, addr := range config.GlobalConfig.ChainEnemies[chainName] {
			enemiesMap[addr] = true
		}

		pairSymbolMap := make(map[string]string)
		for s, pair := range config.GlobalConfig.ChainPairs[chainName] {
			s = strings.Split(s, "@")[1]
			s = strings.ToLower(s)
			pairSymbolMap[pair.Contract] = s
		}

		nodeMgr := core.NewNodeMgr(v.Endpoint, v.WsEndpoint, signals, newRouter, addrTokens, enemiesMap, pairSymbolMap)
		go nodeMgr.Execute()
	}

	select {}
}

type Decoder interface {
	DecodeInputData(params []interface{})
}
