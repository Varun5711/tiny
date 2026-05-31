package logger

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger struct {
	zap     *zap.Logger
	sugar   *zap.SugaredLogger
	service string
}

func New(service string) *Logger {
	var cfg zap.Config

	format := os.Getenv("LOG_FORMAT")
	if strings.ToLower(format) == "text" {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "timestamp"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	}

	envLevel := os.Getenv("LOG_LEVEL")
	switch strings.ToUpper(envLevel) {
	case "DEBUG":
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "WARN":
		cfg.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	case "ERROR":
		cfg.Level = zap.NewAtomicLevelAt(zap.ErrorLevel)
	default:
		cfg.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	}

	z, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}

	z = z.With(zap.String("service", service))

	return &Logger{
		zap:     z,
		sugar:   z.Sugar(),
		service: service,
	}
}

func (l *Logger) With(key string, value interface{}) *Logger {
	newZap := l.zap.With(zap.Any(key, value))
	return &Logger{
		zap:     newZap,
		sugar:   newZap.Sugar(),
		service: l.service,
	}
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.sugar.Debugf(format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.sugar.Infof(format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.sugar.Warnf(format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.sugar.Errorf(format, args...)
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.sugar.Fatalf(format, args...)
}

func (l *Logger) Sync() error {
	return l.zap.Sync()
}

func (l *Logger) Zap() *zap.Logger {
	return l.zap
}
