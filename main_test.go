package main

import (
	"DexExecutorgo/core"
	"encoding/json"
	"fmt"
	"github.com/ethereum/go-ethereum/ethclient"
	"os"
	"testing"
)

type TraceCallResTest struct {
	Jsonrpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Result  struct {
		From         string `json:"from"`
		Gas          string `json:"gas"`
		GasUsed      string `json:"gasUsed"`
		To           string `json:"to"`
		Input        string `json:"input"`
		Output       string `json:"output"`
		Error        string `json:"error"`
		RevertReason string `json:"revertReason"`
		Calls        []struct {
			From         string `json:"from"`
			Gas          string `json:"gas"`
			GasUsed      string `json:"gasUsed"`
			To           string `json:"to"`
			Input        string `json:"input"`
			Output       string `json:"output"`
			Error        string `json:"error"`
			RevertReason string `json:"revertReason"`
			Calls        []struct {
				From    string `json:"from"`
				Gas     string `json:"gas"`
				GasUsed string `json:"gasUsed"`
				To      string `json:"to"`
				Input   string `json:"input"`
				Output  string `json:"output"`
				Calls   []struct {
					From    string `json:"from"`
					Gas     string `json:"gas"`
					GasUsed string `json:"gasUsed"`
					To      string `json:"to"`
					Input   string `json:"input"`
					Output  string `json:"output,omitempty"`
					Value   string `json:"value,omitempty"`
					Type    string `json:"type"`
					Calls   []struct {
						From    string `json:"from"`
						Gas     string `json:"gas"`
						GasUsed string `json:"gasUsed"`
						To      string `json:"to"`
						Input   string `json:"input"`
						Output  string `json:"output,omitempty"`
						Value   string `json:"value"`
						Type    string `json:"type"`
						Calls   []struct {
							From    string `json:"from"`
							Gas     string `json:"gas"`
							GasUsed string `json:"gasUsed"`
							To      string `json:"to"`
							Input   string `json:"input"`
							Output  string `json:"output"`
							Calls   []struct {
								From    string `json:"from"`
								Gas     string `json:"gas"`
								GasUsed string `json:"gasUsed"`
								To      string `json:"to"`
								Input   string `json:"input"`
								Output  string `json:"output"`
								Value   string `json:"value"`
								Type    string `json:"type"`
							} `json:"calls"`
							Value string `json:"value"`
							Type  string `json:"type"`
						} `json:"calls,omitempty"`
					} `json:"calls,omitempty"`
				} `json:"calls"`
				Value string `json:"value"`
				Type  string `json:"type"`
			} `json:"calls"`
			Value string `json:"value"`
			Type  string `json:"type"`
		} `json:"calls"`
		Value string `json:"value"`
		Type  string `json:"type"`
	} `json:"result"`
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
	var result TraceCallResTest
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
	err = ethClient.Client().Call(&result, "debug_traceCall", paramsa, "latest", paramsb)

	tmp, _ := json.Marshal(result)
	var resTraceCall TraceCallResTest
	err = json.Unmarshal(tmp, &resTraceCall)
	if err != nil {
		fmt.Println(resTraceCall)
	} else {
		fmt.Println(err)
	}
}
