package logger

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sjlit/aeus/metadata"
)

type logger struct {
	log *slog.Logger
}

func (l *logger) Debug(ctx context.Context, msg string, args ...any) {
	args = appendTraceArgs(ctx, args)
	l.log.DebugContext(ctx, msg, args...)
}

func (l *logger) Debugf(ctx context.Context, msg string, args ...any) {
	args = appendTraceArgs(ctx, args)
	l.log.DebugContext(ctx, fmt.Sprintf(msg, args...))
}

func (l *logger) Info(ctx context.Context, msg string, args ...any) {
	args = appendTraceArgs(ctx, args)
	l.log.InfoContext(ctx, msg, args...)
}

func (l *logger) Infof(ctx context.Context, msg string, args ...any) {
	args = appendTraceArgs(ctx, args)
	l.log.InfoContext(ctx, fmt.Sprintf(msg, args...))
}

func (l *logger) Warn(ctx context.Context, msg string, args ...any) {
	args = appendTraceArgs(ctx, args)
	l.log.WarnContext(ctx, msg, args...)
}

func (l *logger) Warnf(ctx context.Context, msg string, args ...any) {
	args = appendTraceArgs(ctx, args)
	l.log.WarnContext(ctx, fmt.Sprintf(msg, args...))
}

func (l *logger) Error(ctx context.Context, msg string, args ...any) {
	args = appendTraceArgs(ctx, args)
	l.log.ErrorContext(ctx, msg, args...)
}

func (l *logger) Errorf(ctx context.Context, msg string, args ...any) {
	args = appendTraceArgs(ctx, args)
	l.log.ErrorContext(ctx, fmt.Sprintf(msg, args...))
}

func (l *logger) With(args ...any) Logger {
	return New(l.log.With(args...))
}

// New wraps a *slog.Logger so it satisfies Logger.
func New(log *slog.Logger) Logger {
	return &logger{
		log: log,
	}
}

func appendTraceArgs(ctx context.Context, args []any) []any {
	if ctx == nil {
		return args
	}
	md := metadata.FromContext(ctx)
	tid, hasTid := md.Get("trace_id")
	sid, hasSid := md.Get("span_id")
	if (!hasTid || tid == "") && (!hasSid || sid == "") {
		return args
	}
	// allocate a new slice to avoid mutating the caller's args
	out := make([]any, len(args), len(args)+4)
	copy(out, args)
	if hasTid && tid != "" {
		out = append(out, "trace_id", tid)
	}
	if hasSid && sid != "" {
		out = append(out, "span_id", sid)
	}
	return out
}
