package main

import (
	"DexExecutorgo/core"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"os"
	"testing"
)

type TraceCallResTestB struct {
	From    string              `json:"from"`
	Gas     string              `json:"gas"`
	GasUsed string              `json:"gasUsed"`
	To      string              `json:"to"`
	Input   string              `json:"input"`
	Output  string              `json:"output"`
	Value   string              `json:"value,omitempty"`
	Type    string              `json:"type"`
	Call    []TraceCallResTestB `json:"calls,omitempty"`
}

type DebugTraceCallReq struct {
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	Id      int           `json:"id"`
	Jsonrpc string        `json:"jsonrpc"`
}

func TestRouter(t *testing.T) {
	content, err := os.ReadFile("./test.txt")
	if err != nil {
		fmt.Println(err)
	}

	var (
		pendingTx core.PendingTx
	)
	err = json.Unmarshal(content, &pendingTx)
	if err != nil {
		return
	}

	addrTokens := map[string]string{
		"0xB31f66AA3C1e785363F0875A1B74E27b85FD66c7": "avax",
		"0x49D5c2BdFfac6CE2BFdB6640F4F80f226bc10bAB": "eth",
		"0x152b9d0FdC40C096757F570A51E494bd4b943E50": "btc",
		"0xB97EF9Ef8734C71904D8002F8b6Bc66Dd9c48a6E": "usdc",
		"0x9702230A8Ea53601f5cD2dc00fDBc13d4dF4A8c7": "usdt",
	}
	rDecoder, err := core.NewRouterDecoder(addrTokens)
	if err != nil {
		return
	}

	rDecoder.DecodeToSymbol("avaxuniswap", &pendingTx)
}

func TestDebugTraceCall(t *testing.T) {
	rpcUrl := "http://10.9.1.97:9650/ext/bc/C/rpc"
	content, err := os.ReadFile("./test.txt")
	if err != nil {
		fmt.Println(err)
	}

	var (
		pendingTx core.PendingTx
	)
	err = json.Unmarshal(content, &pendingTx)
	if err != nil {
		return
	}

	//var result core.TraceCallRes
	var result interface{}
	var (
		paramsa = core.ParamsA{
			From: pendingTx.Params.Result.From,
			To:   pendingTx.Params.Result.To,
			Data: pendingTx.Params.Result.Input,
		}

		paramsb = core.Tracer{
			Tracer: "callTracer",
			TracerConfig: core.TracerConfig{
				WithLog:     true,
				OnlyTopCall: false,
			},
		}

		addresses []string
	)

	ethClient, err := ethclient.Dial(rpcUrl)
	err = ethClient.Client().Call(&result, "debug_traceCall", paramsa, "latest", paramsb)

	tmp, _ := json.Marshal(result)
	var resTraceCall TraceCallResTestB
	err = json.Unmarshal(tmp, &resTraceCall)
	if err == nil {
		GetAllAddress([]TraceCallResTestB{resTraceCall}, addresses)
		fmt.Println(addresses)
	} else {
		fmt.Println("22222")
		fmt.Println(err)
	}
}

func GetAllAddress(callsRes []TraceCallResTestB, addresses []string) {
	if len(callsRes) != 0 {
		for _, call := range callsRes {
			addresses = append(addresses, call.To)
			GetAllAddress(call.Call, addresses)
		}
	}
}
