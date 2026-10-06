package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New creates a production-grade zap logger.
// level: debug | info | warn | error
// output: stdout | stderr | /path/to/file.log
func New(level, output string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	var sink zapcore.WriteSyncer
	switch output {
	case "stdout", "":
		sink = zapcore.AddSync(os.Stdout)
	case "stderr":
		sink = zapcore.AddSync(os.Stderr)
	default:
		f, err := os.OpenFile(output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("opening log file %q: %w", output, err)
		}
		sink = zapcore.AddSync(f)
	}

	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		sink,
		zap.NewAtomicLevelAt(zapLevel),
	)
	return zap.New(core, zap.AddCaller()), nil
}
