// Package logger provides a thin abstraction over a structured logger.
// The rest of the application depends only on the Logger interface, so the
// underlying implementation (currently zap) can be swapped without touching
// call sites.
package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Field is a strongly typed key/value pair attached to a log entry.
type Field = zap.Field

// Convenience constructors re-exported so callers do not import zap directly.
var (
	String   = zap.String
	Int      = zap.Int
	Int64    = zap.Int64
	Float64  = zap.Float64
	Bool     = zap.Bool
	Any      = zap.Any
	Err      = zap.Error
	Duration = zap.Duration
)

// Logger is the application-wide logging contract.
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	With(fields ...Field) Logger
	Sync() error
}

type zapLogger struct {
	l *zap.Logger
}

// New builds a Logger. When production is true it emits JSON suitable for log
// aggregation; otherwise it emits human friendly console output.
func New(level string, production bool) (Logger, error) {
	var cfg zap.Config
	if production {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	if lvl, err := zapcore.ParseLevel(level); err == nil {
		cfg.Level = zap.NewAtomicLevelAt(lvl)
	}

	l, err := cfg.Build(zap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}
	return &zapLogger{l: l}, nil
}

func (z *zapLogger) Debug(msg string, fields ...Field) { z.l.Debug(msg, fields...) }
func (z *zapLogger) Info(msg string, fields ...Field)  { z.l.Info(msg, fields...) }
func (z *zapLogger) Warn(msg string, fields ...Field)  { z.l.Warn(msg, fields...) }
func (z *zapLogger) Error(msg string, fields ...Field) { z.l.Error(msg, fields...) }
func (z *zapLogger) Fatal(msg string, fields ...Field) { z.l.Fatal(msg, fields...) }

func (z *zapLogger) With(fields ...Field) Logger {
	return &zapLogger{l: z.l.With(fields...)}
}

func (z *zapLogger) Sync() error { return z.l.Sync() }
