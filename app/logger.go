package app

import (
	"log/slog"
	"os"

	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
	"golang.org/x/term"
)

var core zapcore.Core

func InitLogger(config *Config) {
	var encoderConfig = zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.EpochTimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	isTerminal := term.IsTerminal(int(os.Stdout.Fd()))
	var encoder zapcore.Encoder
	switch {
	case isTerminal:
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	case !isTerminal:
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	core = zapcore.NewCore(encoder, os.Stdout, zapcore.Level(config.LogLevel))
	slog.SetDefault(slog.New(zapslog.NewHandler(core, zapslog.WithCaller(true), zapslog.WithName("isgate"))))
}

func GetLogger(name string, opts ...zapslog.HandlerOption) *slog.Logger {
	defaultOpts := []zapslog.HandlerOption{
		zapslog.WithCaller(true),
		zapslog.WithName(name),
	}
	return slog.New(zapslog.NewHandler(core, append(defaultOpts, opts...)...))
}
