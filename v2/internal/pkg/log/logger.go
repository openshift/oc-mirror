package log

import (
	"fmt"

	"github.com/microlib/simple"
)

// PluggableLoggerInterface - allows us to use other logging systems
// as long as the interface implementation is adhered to
type PluggableLoggerInterface interface {
	Error(msg string, val ...interface{})
	Info(msg string, val ...interface{})
	Debug(msg string, val ...interface{})
	Trace(msg string, val ...interface{})
	Warn(msg string, val ...interface{})
	Level(levele string)
}

// PluggableLogger
type PluggableLogger struct {
	Log *simple.Logger
}

// New - returns a new PluggableLogger instance
func New(level string) PluggableLoggerInterface {
	return &PluggableLogger{Log: &simple.Logger{Level: level}}
}

// Error
func (c *PluggableLogger) Error(msg string, val ...interface{}) {
	c.Log.Error(fmt.Sprintf(msg, val...))
}

// Info
func (c *PluggableLogger) Info(msg string, val ...interface{}) {
	c.Log.Info(fmt.Sprintf(msg, val...))
}

// Debug
func (c *PluggableLogger) Debug(msg string, val ...interface{}) {
	c.Log.Debug(fmt.Sprintf(msg, val...))
}

// Trace
func (c *PluggableLogger) Trace(msg string, val ...interface{}) {
	c.Log.Trace(fmt.Sprintf(msg, val...))
}

// Warn
func (c *PluggableLogger) Warn(msg string, val ...interface{}) {
	c.Log.Warn(fmt.Sprintf(msg, val...))
}

// Level - ovveride log level
func (c *PluggableLogger) Level(level string) {
	c.Log.Level = level
}
