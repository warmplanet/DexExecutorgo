package core

import (
	"github.com/ethereum/go-ethereum/common"
	"math/big"
)

type wsRequest struct {
	Id      int      `json:"id"`
	JsonRpc float64  `json:"jsonrpc"`
	Method  string   `json:"method"`
	Params  []string `json:"params"`
}

type PendingTx struct {
	Type                 string        `json:"type"`
	ChainId              string        `json:"chainId"`
	Nonce                string        `json:"nonce"`
	From                 string        `json:"from"`
	To                   string        `json:"to"`
	Gas                  string        `json:"gas"`
	GasPrice             interface{}   `json:"gasPrice"`
	MaxPriorityFeePerGas string        `json:"maxPriorityFeePerGas"`
	MaxFeePerGas         string        `json:"maxFeePerGas"`
	Value                string        `json:"value"`
	Input                string        `json:"input"`
	AccessList           []interface{} `json:"accessList"`
	V                    string        `json:"v"`
	R                    string        `json:"r"`
	S                    string        `json:"s"`
	YParity              string        `json:"yParity"`
	Hash                 string        `json:"hash"`
}

type TraderJoePath struct {
	PairBinSteps []*big.Int       `json:"pairBinSteps"`
	Versions     []uint8          `json:"versions"`
	TokenPath    []common.Address `json:"tokenPath"`
}

type HeadRsp struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  struct {
		Subscription string `json:"subscription"`
		Result       struct {
			ParentHash            string      `json:"parentHash"`
			Sha3Uncles            string      `json:"sha3Uncles"`
			Miner                 string      `json:"miner"`
			StateRoot             string      `json:"stateRoot"`
			TransactionsRoot      string      `json:"transactionsRoot"`
			ReceiptsRoot          string      `json:"receiptsRoot"`
			LogsBloom             string      `json:"logsBloom"`
			Difficulty            string      `json:"difficulty"`
			Number                int64       `json:"number"`
			GasLimit              string      `json:"gasLimit"`
			GasUsed               string      `json:"gasUsed"`
			Timestamp             int64       `json:"timestamp"`
			ExtraData             string      `json:"extraData"`
			MixHash               string      `json:"mixHash"`
			Nonce                 string      `json:"nonce"`
			ExtDataHash           string      `json:"extDataHash"`
			BaseFeePerGas         string      `json:"baseFeePerGas"`
			ExtDataGasUsed        string      `json:"extDataGasUsed"`
			BlockGasCost          string      `json:"blockGasCost"`
			BlobGasUsed           interface{} `json:"blobGasUsed"`
			ExcessBlobGas         interface{} `json:"excessBlobGas"`
			ParentBeaconBlockRoot interface{} `json:"parentBeaconBlockRoot"`
			Hash                  string      `json:"hash"`
		} `json:"result"`
	} `json:"params"`
}

type SignalPath struct {
	Price       float64 `json:"price"`
	Amount      float64 `json:"amount"`
	Symbol      string  `json:"symbol"`
	PriceAvg    float64 `json:"price_avg"`
	AmountSum   float64 `json:"amount_sum"`
	Exchange    int     `json:"exchange"`
	Market      int     `json:"market"`
	SymbolType  int     `json:"symbol_type"`
	PriceExpect float64 `json:"price_expect"`
}

type Signal struct {
	Server    string `json:"server"`
	DexMarket string `json:"dex_market"`
	Symbol    string `json:"symbol"`
	BuildTx   struct {
		Nonce                int    `json:"nonce"`
		From                 string `json:"from"`
		To                   string `json:"to"`
		Value                int    `json:"value"`
		Gas                  int    `json:"gas"`
		GasPrice             int64  `json:"gasPrice"`
		MaxPriorityFeePerGas int64  `json:"maxPriorityFeePerGas"`
		MaxFeePerGas         int64  `json:"maxFeePerGas"`
		Data                 string `json:"data"`
		ChainId              int    `json:"chainId"`
	} `json:"build_tx"`
	UpdateTime      float64 `json:"updatetime"`
	SignalBlockNum  int     `json:"signal_blocknum"`
	SignalBlockTime float64 `json:"signal_blocktime"`
	MaxGasPrice     float64 `json:"max_gas_price"`
	BaseGasPrice    float64 `json:"base_gas_price"`
	TradeBlockNum   int64   `json:"trade_blocknum"`
	RegistId        int64   `json:"regist_id"`
	HedgeId         int64   `json:"hedge_id"`
	DepthDetails    map[string][]struct {
		Exchange      int          `json:"exchange"`
		PriceAvg      float64      `json:"price_avg"`
		AmountSum     float64      `json:"amount_sum"`
		AmountUsdt    float64      `json:"amount_usdt"`
		Path          []SignalPath `json:"path"`
		LastAmountSum float64      `json:"last_amount_sum"`
		LastPriceAvg  float64      `json:"last_price_avg"`
		Amount        float64      `json:"amount"`
		Price         float64      `json:"price"`
		AmountConsume float64      `json:"amount_consume"`
		TotalRatio    float64      `json:"total_ratio"`
		ConsumeRatio  float64      `json:"consume_ratio"`
		PriceExpect   float64      `json:"price_expect"`
	} `json:"depth_details"`
	SymbolsTradeSide map[string]map[string]string `json:"symbols_trade_side"`
	PriceExpect      float64                      `json:"price_expect"`
	Side             string                       `json:"side"`
	AmountArb        float64                      `json:"amount_arb"`
	AmountEntry      float64                      `json:"amount_entry"`
}

type DebugTraceCallReq struct {
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	Id      int           `json:"id"`
	Jsonrpc string        `json:"jsonrpc"`
}

type ParamsA struct {
	From string `json:"from"`
	To   string `json:"to"`
	Data string `json:"data"`
}

type TracerConfig struct {
	WithLog     bool `json:"withLog"`
	OnlyTopCall bool `json:"onlyTopCall"`
}

type Tracer struct {
	Tracer       string       `json:"tracer"`
	TracerConfig TracerConfig `json:"tracerConfig"`
}

type CallsRes struct {
	From    string     `json:"from"`
	Gas     string     `json:"gas"`
	GasUsed string     `json:"gasUsed"`
	To      string     `json:"to"`
	Input   string     `json:"input"`
	Output  string     `json:"output"`
	Value   string     `json:"value,omitempty"`
	Type    string     `json:"type"`
	Call    []CallsRes `json:"calls,omitempty"`
}

type TraceCallRes struct {
	Jsonrpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Result  struct {
		From    string   `json:"from"`
		Gas     string   `json:"gas"`
		GasUsed string   `json:"gasUsed"`
		To      string   `json:"to"`
		Input   string   `json:"input"`
		Output  string   `json:"output"`
		Error   string   `json:"error"`
		Calls   CallsRes `json:"calls"`
		Value   string   `json:"value"`
		Type    string   `json:"type"`
	} `json:"result"`
}
