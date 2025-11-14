package logger

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// Logger wraps slog.Logger to provide a compatible interface
type Logger struct {
	*slog.Logger
}

// NewLogger creates a new logger with the specified level
// Defaults to text format for better readability
func NewLogger(level string) *Logger {
	return NewTextLogger(level)
}

// NewTextLogger creates a new text-based logger
// This is the default format for better human readability
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

// NewJSONLogger creates a new JSON-based logger
// Useful for production environments and log aggregation
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

// Log is a compatibility method that mimics the go-kit/log interface
func (l *Logger) Log(keyvals ...interface{}) error {
	if len(keyvals)%2 != 0 {
		keyvals = append(keyvals, "MISSING")
	}

	args := make([]interface{}, 0, len(keyvals))
	for i := 0; i < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			continue
		}
		args = append(args, key, keyvals[i+1])
	}

	l.Logger.Info("", args...)
	return nil
}

// With returns a new logger with the given key-value pairs added to the context
func (l *Logger) With(keyvals ...interface{}) *Logger {
	if len(keyvals)%2 != 0 {
		keyvals = append(keyvals, "MISSING")
	}

	args := make([]interface{}, 0, len(keyvals))
	for i := 0; i < len(keyvals); i += 2 {
		key, ok := keyvals[i].(string)
		if !ok {
			continue
		}
		args = append(args, key, keyvals[i+1])
	}

	return &Logger{Logger: l.Logger.With(args...)}
}

// Debug logs a debug message
func (l *Logger) Debug(msg string, args ...interface{}) {
	l.Logger.Debug(msg, args...)
}

// Info logs an info message
func (l *Logger) Info(msg string, args ...interface{}) {
	l.Logger.Info(msg, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(msg string, args ...interface{}) {
	l.Logger.Warn(msg, args...)
}

// Error logs an error message
func (l *Logger) Error(msg string, args ...interface{}) {
	l.Logger.Error(msg, args...)
}

// WithContext returns a logger with the given context
// Note: slog doesn't have WithContext, so this is a no-op for compatibility
func (l *Logger) WithContext(ctx context.Context) *Logger {
	return l
}

// WithTimeout adds timeout information to the logger context
func (l *Logger) WithTimeout(timeout time.Duration) *Logger {
	return l.With("timeout", timeout)
}

// WithCommand adds command information to the logger context
func (l *Logger) WithCommand(command string, args []string) *Logger {
	return l.With("command", command, "args", args)
}
