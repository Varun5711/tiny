package logger

import (
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
	return newLogger(service, nil)
}

func NewWithSyncer(service string, extraSyncer zapcore.WriteSyncer) *Logger {
	return newLogger(service, extraSyncer)
}

func newLogger(service string, extraSyncer zapcore.WriteSyncer) *Logger {
	var level zapcore.Level
	envLevel := os.Getenv("LOG_LEVEL")
	switch strings.ToUpper(envLevel) {
	case "DEBUG":
		level = zap.DebugLevel
	case "WARN":
		level = zap.WarnLevel
	case "ERROR":
		level = zap.ErrorLevel
	default:
		level = zap.InfoLevel
	}

	var encoder zapcore.Encoder
	format := os.Getenv("LOG_FORMAT")
	if strings.ToLower(format) == "text" {
		encCfg := zap.NewDevelopmentEncoderConfig()
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	} else {
		encCfg := zap.NewProductionEncoderConfig()
		encCfg.TimeKey = "timestamp"
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder = zapcore.NewJSONEncoder(encCfg)
	}

	cores := []zapcore.Core{
		zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level),
	}

	if extraSyncer != nil {
		jsonEnc := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
		cores = append(cores, zapcore.NewCore(jsonEnc, extraSyncer, level))
	}

	core := zapcore.NewTee(cores...)
	z := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
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
