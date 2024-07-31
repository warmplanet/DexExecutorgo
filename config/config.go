package config

import (
	cliConfig "clients/config"
	"github.com/warmplanet/proto/go/sdk"
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
}
