package main

import (
	"DexExecutorgo/core"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"os"
	"testing"
)

type TestPendingTx struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Subscription string `json:"subscription"`
		Result       struct {
			BlockHash            string        `json:"blockHash"`
			BlockNumber          string        `json:"blockNumber"`
			From                 string        `json:"from"`
			Gas                  string        `json:"gas"`
			GasPrice             string        `json:"gasPrice"`
			MaxFeePerGas         string        `json:"maxFeePerGas"`
			MaxPriorityFeePerGas string        `json:"maxPriorityFeePerGas"`
			Hash                 string        `json:"hash"`
			Input                string        `json:"input"`
			Nonce                string        `json:"nonce"`
			To                   string        `json:"to"`
			TransactionIndex     string        `json:"transactionIndex"`
			Value                string        `json:"value"`
			Type                 string        `json:"type"`
			AccessList           []interface{} `json:"accessList"`
			ChainId              string        `json:"chainId"`
			V                    string        `json:"v"`
			R                    string        `json:"r"`
			S                    string        `json:"s"`
			YParity              string        `json:"yParity"`
		} `json:"result"`
	} `json:"params"`
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
	rpcUrl := "https://avalanche.drpc.org"
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

	var result core.TraceCallRes
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
		req = DebugTraceCallReq{
			Method:  "debug_traceCall",
			Params:  []interface{}{paramsa, "latest", paramsb},
			Id:      1,
			Jsonrpc: "2.0",
		}
	)

	reqByte, _ := json.Marshal(req)
	fmt.Println(string(reqByte))
	ethClient, err := ethclient.Dial(rpcUrl)
	err = ethClient.Client().Call(result, "debug_traceCall", paramsa, "latest", paramsb)
	fmt.Println(result, err)
}
