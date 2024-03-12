package log

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
