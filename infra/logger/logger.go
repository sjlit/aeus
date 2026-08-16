package logger

import (
	"context"
	"log/slog"
	"os"
)

var (
	log Logger
)

func init() {
	log = New(slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}),
	))
}

type Logger interface {
	With(args ...any) Logger
	Debug(ctx context.Context, format string, args ...any) //Structured context as loosely typed key-value pairs.
	Debugf(ctx context.Context, format string, args ...any)
	Info(ctx context.Context, format string, args ...any)
	Infof(ctx context.Context, format string, args ...any)
	Warn(ctx context.Context, format string, args ...any)
	Warnf(ctx context.Context, format string, args ...any)
	Error(ctx context.Context, format string, args ...any)
	Errorf(ctx context.Context, format string, args ...any)
}

func Debug(ctx context.Context, format string, args ...any) {
	log.Debug(ctx, format, args...)
}

func Debugf(ctx context.Context, format string, args ...any) {
	log.Debugf(ctx, format, args...)
}

func Info(ctx context.Context, format string, args ...any) {
	log.Info(ctx, format, args...)
}

func Infof(ctx context.Context, format string, args ...any) {
	log.Infof(ctx, format, args...)
}

func Warn(ctx context.Context, format string, args ...any) {
	log.Warn(ctx, format, args...)
}

func Warnf(ctx context.Context, format string, args ...any) {
	log.Warnf(ctx, format, args...)
}

func Error(ctx context.Context, format string, args ...any) {
	log.Error(ctx, format, args...)
}

func Errorf(ctx context.Context, format string, args ...any) {
	log.Errorf(ctx, format, args...)
}

func Default() Logger {
	return log
}
