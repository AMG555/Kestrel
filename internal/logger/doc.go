// Package logger wraps go.uber.org/zap with structured JSON output, configurable
// log level, file or stdout sinks, and a daily-rotating diagnostic writer that
// captures warn-and-above entries to a separate per-conversation log directory.
package logger
