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
	l.log.DebugContext(ctx, msg, appendTraceArgs(ctx, args)...)
}

func (l *logger) Debugf(ctx context.Context, format string, args ...any) {
	l.log.DebugContext(ctx, fmt.Sprintf(format, args...), traceArgs(ctx)...)
}

func (l *logger) Info(ctx context.Context, msg string, args ...any) {
	l.log.InfoContext(ctx, msg, appendTraceArgs(ctx, args)...)
}

func (l *logger) Infof(ctx context.Context, format string, args ...any) {
	l.log.InfoContext(ctx, fmt.Sprintf(format, args...), traceArgs(ctx)...)
}

func (l *logger) Warn(ctx context.Context, msg string, args ...any) {
	l.log.WarnContext(ctx, msg, appendTraceArgs(ctx, args)...)
}

func (l *logger) Warnf(ctx context.Context, format string, args ...any) {
	l.log.WarnContext(ctx, fmt.Sprintf(format, args...), traceArgs(ctx)...)
}

func (l *logger) Error(ctx context.Context, msg string, args ...any) {
	l.log.ErrorContext(ctx, msg, appendTraceArgs(ctx, args)...)
}

func (l *logger) Errorf(ctx context.Context, format string, args ...any) {
	l.log.ErrorContext(ctx, fmt.Sprintf(format, args...), traceArgs(ctx)...)
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

func traceArgs(ctx context.Context) []any {
	if ctx == nil {
		return nil
	}
	md := metadata.FromContext(ctx)
	tid, hasTid := md.Get("trace_id")
	sid, hasSid := md.Get("span_id")
	if (!hasTid || tid == "") && (!hasSid || sid == "") {
		return nil
	}
	out := make([]any, 0, 4)
	if hasTid && tid != "" {
		out = append(out, "trace_id", tid)
	}
	if hasSid && sid != "" {
		out = append(out, "span_id", sid)
	}
	return out
}

func appendTraceArgs(ctx context.Context, args []any) []any {
	trace := traceArgs(ctx)
	if len(trace) == 0 {
		return args
	}
	// allocate a new slice to avoid mutating the caller's args
	out := make([]any, 0, len(args)+len(trace))
	out = append(out, args...)
	out = append(out, trace...)
	return out
}
