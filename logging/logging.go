// Package logging connects utility diagnostics to an application-owned logger.
// It does not choose a logging framework or configure process-wide output.
package logging

import "sync/atomic"

type Level uint8

const (
	Debug Level = iota
	Info
	Warn
	Error
)

// Field is a structured diagnostic value. Values retain their original types.
type Field struct {
	Key   string
	Value any
}

// Logger receives diagnostics synchronously and must support concurrent calls.
// Implementations control filtering, formatting, and output.
type Logger interface {
	Log(level Level, message string, fields ...Field)
}

type loggerHolder struct{ logger Logger }

var current atomic.Pointer[loggerHolder]

// SetLogger installs the application logger. A nil logger disables diagnostics,
// which is also the default. It may be called concurrently with Log.
// In-flight calls may finish using the previously installed logger.
func SetLogger(logger Logger) {
	if logger == nil {
		current.Store(nil)
		return
	}
	current.Store(&loggerHolder{logger})
}

// Log forwards a diagnostic to the configured logger, if any.
func Log(level Level, message string, fields ...Field) {
	if holder := current.Load(); holder != nil {
		holder.logger.Log(level, message, fields...)
	}
}
