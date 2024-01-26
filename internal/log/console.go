package log

import "fmt"

type ConsoleLogger struct {
}

func (c *ConsoleLogger) Debug(msg string, keysAndValues ...interface{}) {
	c.log("debug", msg, keysAndValues)
}
func (c *ConsoleLogger) Info(msg string, keysAndValues ...interface{}) {
	c.log("info", msg, keysAndValues)
}
func (c *ConsoleLogger) Warn(msg string, keysAndValues ...interface{}) {
	c.log("warn", msg, keysAndValues)
}
func (c *ConsoleLogger) Error(msg string, keysAndValues ...interface{}) {
	c.log("error", msg, keysAndValues)
}
func (c *ConsoleLogger) Fatal(msg string, keysAndValues ...interface{}) {
	c.log("error", msg, keysAndValues)
	panic(msg)
}
func (c *ConsoleLogger) log(level string, msg string, keysAndValues ...interface{}) {
	var kvStr string
	for _, field := range keysAndValues {
		kvStr += fmt.Sprint(field) + " "
	}

	fmt.Println(level, "-", msg, kvStr)
}
