// Package logger provides a thin, opinionated wrapper around Uber's Zap
// logging library. It exposes a Printf-style API (Debug/Info/Warn/Error/Fatal)
// instead of Zap's structured-field API so that callers throughout the
// codebase can log with minimal ceremony while still getting the performance
// benefits of Zap under the hood.
//
// Configuration is driven by two environment variables:
//
//	LOG_LEVEL  -- DEBUG, WARN, ERROR; defaults to INFO.
//	LOG_FORMAT -- "text" for human-readable console output;
//	              anything else (including empty) produces JSON,
//	              which is what the production ELK stack expects.
//
// An optional extra WriteSyncer can be provided at construction time to
// duplicate log output to a secondary sink (e.g., Elasticsearch) without
// changing the caller-facing API.
package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps a Zap logger and its sugared variant, providing a simplified
// Printf-style interface. The service field is baked into every log line so
// that aggregated logs in Elasticsearch can be filtered by microservice name.
type Logger struct {
	zap     *zap.Logger
	sugar   *zap.SugaredLogger
	service string
}

// New creates a Logger that writes to stdout only.
// This is the standard constructor used by most services at startup.
func New(service string) *Logger {
	return newLogger(service, nil)
}

// NewWithSyncer creates a Logger that writes to both stdout and an
// additional WriteSyncer. This is used when logs should also be shipped
// to Elasticsearch in real time via a custom syncer, giving operators
// centralized log visibility without relying solely on container stdout.
func NewWithSyncer(service string, extraSyncer zapcore.WriteSyncer) *Logger {
	return newLogger(service, extraSyncer)
}

// newLogger is the shared constructor. It reads LOG_LEVEL and LOG_FORMAT
// from the environment, builds one or more Zap cores, and returns a
// ready-to-use Logger. AddCaller is enabled so every log line includes
// the source file and line number; CallerSkip(1) compensates for the
// extra stack frame introduced by our wrapper methods.
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

	// LOG_FORMAT=text uses a colorized console encoder suitable for local
	// development. Everything else defaults to JSON for production, which
	// is directly ingestible by Filebeat / Logstash / Elasticsearch.
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

	// The Tee core fans out every log entry to all registered cores.
	// The primary core always writes to stdout; the optional extra core
	// sends JSON-encoded logs to whatever syncer the caller provides.
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

// With returns a child Logger that carries an additional key-value pair in
// every subsequent log line. This is useful for request-scoped fields like
// trace IDs or user IDs without polluting the format string.
func (l *Logger) With(key string, value interface{}) *Logger {
	newZap := l.zap.With(zap.Any(key, value))
	return &Logger{
		zap:     newZap,
		sugar:   newZap.Sugar(),
		service: l.service,
	}
}

// Debug logs a message at DEBUG level using Printf-style formatting.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.sugar.Debugf(format, args...)
}

// Info logs a message at INFO level using Printf-style formatting.
func (l *Logger) Info(format string, args ...interface{}) {
	l.sugar.Infof(format, args...)
}

// Warn logs a message at WARN level using Printf-style formatting.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.sugar.Warnf(format, args...)
}

// Error logs a message at ERROR level using Printf-style formatting.
func (l *Logger) Error(format string, args ...interface{}) {
	l.sugar.Errorf(format, args...)
}

// Fatal logs a message at FATAL level and then calls os.Exit(1).
// Use sparingly -- only for truly unrecoverable startup errors.
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.sugar.Fatalf(format, args...)
}

// Sync flushes any buffered log entries. This should be called (typically
// via defer) before the process exits to avoid losing the final log lines.
func (l *Logger) Sync() error {
	return l.zap.Sync()
}

// Zap returns the underlying *zap.Logger for callers that need the full
// structured logging API or need to pass a Zap logger to a third-party
// library (e.g., gRPC interceptors, OTel bridges).
func (l *Logger) Zap() *zap.Logger {
	return l.zap
}
