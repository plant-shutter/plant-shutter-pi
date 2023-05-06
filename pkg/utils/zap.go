package utils

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger() *zap.SugaredLogger {
	cfg := zap.Config{
		Level:    zap.NewAtomicLevelAt(zapcore.InfoLevel),
		Encoding: "console",
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:  "msg",
			LevelKey:    "level",
			TimeKey:     "time",
			EncodeLevel: zapcore.CapitalLevelEncoder,
			EncodeTime:  zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		},
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}

	l, err := cfg.Build()
	if err != nil {
		panic(err)
	}
	return l.Sugar()
}
