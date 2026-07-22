package logger

import (
	"log/slog"
	"os"
)

// Logger embeds *slog.Logger and is the logging dependency passed to every
// collector, so collectors never reach for a global logger.
type Logger struct {
	*slog.Logger
}

// NewLogger creates a logger with the specified level, defaulting to text format.
func NewLogger(level string) *Logger {
	return NewTextLogger(level)
}

// NewTextLogger creates a text-format logger suitable for interactive use.
func NewTextLogger(level string) *Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove the source file path for cleaner output
			if a.Key == slog.SourceKey {
				return slog.Attr{}
			}
			return a
		},
	}

	handler := slog.NewTextHandler(os.Stdout, opts)
	logger := slog.New(handler)

	return &Logger{Logger: logger}
}

// NewJSONLogger creates a JSON-format logger suitable for log aggregation.
func NewJSONLogger(level string) *Logger {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     slogLevel,
		AddSource: true,
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)
	logger := slog.New(handler)

	return &Logger{Logger: logger}
}

func (l *Logger) Debug(msg string, args ...interface{}) {
	l.Logger.Debug(msg, args...)
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.Logger.Info(msg, args...)
}

func (l *Logger) Warn(msg string, args ...interface{}) {
	l.Logger.Warn(msg, args...)
}

func (l *Logger) Error(msg string, args ...interface{}) {
	l.Logger.Error(msg, args...)
}
