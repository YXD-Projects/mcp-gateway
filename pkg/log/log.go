package log

import (
	"go.uber.org/zap"
)

var Logger *zap.Logger

func Init() {
	if Logger == nil {
		l, err := zap.NewProduction()
		if err != nil {
			panic(err)
		}
		Logger = l
	}
}

func Sync() {
	if Logger != nil {
		_ = Logger.Sync()
	}
}
