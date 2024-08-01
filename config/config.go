package config

import (
	cliConfig "clients/config"
	"github.com/goccy/go-json"
	"github.com/warmplanet/proto/go/sdk"
	"io/ioutil"
	"os"
	"strings"
)

var (
	GlobalConfig *DexExecutorConfig
)

type DexExecutorConfig struct {
	LogPath      string                            `toml:"log_path"`
	LogLevel     string                            `toml:"log_level"`
	Env          string                            `toml:"env"`
	IsTest       bool                              `toml:"is_test"`
	ProxyUrl     string                            `toml:"proxy"` // 代理
	NatsConfig   *sdk.TtNatsConfig                 `toml:"nats"`
	ChainConfig  map[string]*cliConfig.TtChainItem `toml:"chain"`
	ChainEnemies map[string][]string               `toml:"enemies"`
	ChainPairs   map[string]map[string]Pairs
}

type Pairs struct {
	Contract                 string  `json:"contract"`
	Token0                   string  `json:"token0"`
	Token1                   string  `json:"token1"`
	ReverseTokenOrder        bool    `json:"reverse_token_order"`
	RawFee                   int     `json:"raw_fee"`
	Fee                      float64 `json:"fee"`
	Proto                    string  `json:"proto"`
	Factory                  string  `json:"factory"`
	Index                    int     `json:"index"`
	TickSpacing              int     `json:"tick_spacing"`
	BinStep                  int     `json:"bin_step"`
	BaseFactor               int     `json:"base_factor"`
	FilterPeriod             int     `json:"filter_period"`
	DecayPeriod              int     `json:"decay_period"`
	ReductionFactor          int     `json:"reduction_factor"`
	VariableFeeControl       int     `json:"variable_fee_control"`
	MaxVolatilityAccumulator int     `json:"max_volatility_accumulator"`
}

func init() {
	LoadConfigFile("./conf/config.toml")
}

func LoadConfigFile(file string) {
	if err := sdk.LoadConfigFile(file, &GlobalConfig); err != nil {
		panic(err)
	}
	var c cliConfig.TtChainConfig
	err := cliConfig.LoadConfigFromFile(file, &c)
	if err != nil {
		panic(err)
	}
	GlobalConfig.ChainConfig = c.ChainList

	chainPairsMap := LoadPairsConfig("./conf/pairs")
	GlobalConfig.ChainPairs = chainPairsMap
}

func LoadPairsConfig(pathName string) map[string]map[string]Pairs {
	chainPairsMap := make(map[string]map[string]Pairs)
	rd, err := ioutil.ReadDir(pathName)
	if err != nil {
		panic(err)
	}

	for _, fi := range rd {
		if !fi.IsDir() {
			fileName := fi.Name()
			fullName := pathName + "/" + fileName
			content, readErr := os.ReadFile(fullName)
			if readErr != nil {
				panic(readErr)
			}
			pairsMap := make(map[string]Pairs)
			err = json.Unmarshal(content, &pairsMap)
			if err != nil {
				panic(err)
			}
			chainName := strings.Split(fileName, ".")[0]
			chainPairsMap[chainName] = pairsMap
		}
	}
	return chainPairsMap
}
