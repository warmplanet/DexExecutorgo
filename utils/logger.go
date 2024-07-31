package utils

import (
	"github.com/warmplanet/proto/go/sdk"
	"go.uber.org/zap"
	"time"
)

var (
	Logger    *zap.SugaredLogger
	logName   = "dex_executor.log"
	log_Level = "info"
)

func InitLogger(path string, level string) {
	if path == "" {
		path = "./logs"
	}
	if level == "" {
		level = log_Level
	}
	Logger, _ = sdk.NewLoggerZapByParam(path, logName, level, 256, 100, 7)
	now_ts := time.Now().UnixMicro()
	Logger.Info("Log begin at ", now_ts)
}
