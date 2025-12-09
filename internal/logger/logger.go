package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

var (
	levelNames = map[Level]string{
		DEBUG: "DEBUG",
		INFO:  "INFO",
		WARN:  "WARN",
		ERROR: "ERROR",
		FATAL: "FATAL",
	}

	levelColors = map[Level]string{
		DEBUG: "\033[36m", // Cyan
		INFO:  "\033[32m", // Green
		WARN:  "\033[33m", // Yellow
		ERROR: "\033[31m", // Red
		FATAL: "\033[35m", // Magenta
	}

	reset = "\033[0m"
)

type Logger struct {
	level      Level
	out        io.Writer
	service    string
	useColors  bool
	showTime   bool
	showCaller bool
}

func New(service string) *Logger {
	level := INFO
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		switch strings.ToUpper(envLevel) {
		case "DEBUG":
			level = DEBUG
		case "INFO":
			level = INFO
		case "WARN":
			level = WARN
		case "ERROR":
			level = ERROR
		case "FATAL":
			level = FATAL
		}
	}

	useColors := os.Getenv("LOG_COLORS") != "false"

	return &Logger{
		level:      level,
		out:        os.Stdout,
		service:    service,
		useColors:  useColors,
		showTime:   true,
		showCaller: false,
	}
}

func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	var buf strings.Builder

	// Timestamp
	if l.showTime {
		buf.WriteString(time.Now().Format("15:04:05"))
		buf.WriteString(" ")
	}

	// Level with color
	if l.useColors {
		buf.WriteString(levelColors[level])
	}
	buf.WriteString(fmt.Sprintf("%-5s", levelNames[level]))
	if l.useColors {
		buf.WriteString(reset)
	}
	buf.WriteString(" ")

	// Service name
	if l.service != "" {
		if l.useColors {
			buf.WriteString("\033[90m") // Gray
		}
		buf.WriteString("[")
		buf.WriteString(l.service)
		buf.WriteString("]")
		if l.useColors {
			buf.WriteString(reset)
		}
		buf.WriteString(" ")
	}

	// Message
	msg := fmt.Sprintf(format, args...)
	buf.WriteString(msg)

	// Write to output
	fmt.Fprintln(l.out, buf.String())

	if level == FATAL {
		os.Exit(1)
	}
}

func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
}

// SetStdLog redirects standard log package to use this logger
func (l *Logger) SetStdLog() {
	log.SetOutput(&stdLogWriter{logger: l})
	log.SetFlags(0)
}

type stdLogWriter struct {
	logger *Logger
}

func (w *stdLogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	w.logger.Info(msg)
	return len(p), nil
}
