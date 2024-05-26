package log

import (
	"log/slog"
)

var (
	DefaultLogger = slog.Default()
)

// Logger is an interface that provides methods for logging messages with different severity levels.
type Logger interface {
	// Debug logs a debug message with optional keys and values.
	Debug(msg string, keysAndValues ...any)
	// Info logs an informational message with optional keys and values.
	Info(msg string, keysAndValues ...any)
	// Warn logs a warning message with optional keys and values.
	Warn(msg string, keysAndValues ...any)
	// Error logs an error message with optional keys and values.
	Error(msg string, keysAndValues ...any)
}

func Debug(msg string, keysAndValues ...any) {
	DefaultLogger.Debug(msg, keysAndValues...)
}

func Info(msg string, keysAndValues ...any) {
	DefaultLogger.Info(msg, keysAndValues...)
}

func Warn(msg string, keysAndValues ...any) {
	DefaultLogger.Warn(msg, keysAndValues...)
}

func Error(msg string, keysAndValues ...any) {
	DefaultLogger.Error(msg, keysAndValues...)
}
