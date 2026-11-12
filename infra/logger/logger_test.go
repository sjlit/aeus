package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/sjlit/aeus/metadata"
)

type testLogger struct {
	lastLevel string
	lastMsg   string
}

func (t *testLogger) Debug(ctx context.Context, msg string, args ...any) {
	t.lastLevel = "debug"
	t.lastMsg = msg
}
func (t *testLogger) Debugf(ctx context.Context, format string, args ...any) { t.lastLevel = "debug" }
func (t *testLogger) Info(ctx context.Context, msg string, args ...any) {
	t.lastLevel = "info"
	t.lastMsg = msg
}
func (t *testLogger) Infof(ctx context.Context, format string, args ...any) { t.lastLevel = "info" }
func (t *testLogger) Warn(ctx context.Context, msg string, args ...any) {
	t.lastLevel = "warn"
	t.lastMsg = msg
}
func (t *testLogger) Warnf(ctx context.Context, format string, args ...any) { t.lastLevel = "warn" }
func (t *testLogger) Error(ctx context.Context, msg string, args ...any) {
	t.lastLevel = "error"
	t.lastMsg = msg
}
func (t *testLogger) Errorf(ctx context.Context, format string, args ...any) { t.lastLevel = "error" }

func (t *testLogger) With(args ...any) Logger { return t }

func TestPackageLevel_Error(t *testing.T) {
	tl := &testLogger{}
	log = tl
	Error(context.Background(), "something went wrong")
	if tl.lastLevel != "error" {
		t.Fatalf("expected level error, got %s", tl.lastLevel)
	}
	if tl.lastMsg != "something went wrong" {
		t.Fatalf("unexpected message: %s", tl.lastMsg)
	}
}

func TestFormattedLogsKeepMessageAndTraceFieldsSeparate(t *testing.T) {
	ctx := metadata.NewContext(context.Background(), metadata.New())
	md := metadata.FromContext(ctx)
	md.Set("trace_id", "trace-abc")
	md.Set("span_id", "span-def")

	cases := []struct {
		name string
		log  func(Logger)
	}{
		{"Debugf", func(l Logger) { l.Debugf(ctx, "hello %s", "world") }},
		{"Infof", func(l Logger) { l.Infof(ctx, "hello %s", "world") }},
		{"Warnf", func(l Logger) { l.Warnf(ctx, "hello %s", "world") }},
		{"Errorf", func(l Logger) { l.Errorf(ctx, "hello %s", "world") }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			lg := New(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			tc.log(lg)

			var record map[string]any
			if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
				t.Fatalf("decode log record: %v; raw=%s", err, buf.String())
			}
			msg, _ := record["msg"].(string)
			if msg != "hello world" {
				t.Fatalf("msg = %q, want %q", msg, "hello world")
			}
			if strings.Contains(msg, "%!") {
				t.Fatalf("formatted message contains fmt marker: %q", msg)
			}
			if got := record["trace_id"]; got != "trace-abc" {
				t.Fatalf("trace_id = %v, want trace-abc", got)
			}
			if got := record["span_id"]; got != "span-def" {
				t.Fatalf("span_id = %v, want span-def", got)
			}
		})
	}
}

func TestStructuredLogKeepsCallerArgs(t *testing.T) {
	ctx := metadata.NewContext(context.Background(), metadata.New())
	metadata.FromContext(ctx).Set("trace_id", "trace-abc")

	var buf bytes.Buffer
	lg := New(slog.New(slog.NewJSONHandler(&buf, nil)))
	lg.Info(ctx, "request done", "status", 200)

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("decode log record: %v; raw=%s", err, buf.String())
	}
	if got := record["status"]; got != float64(200) {
		t.Fatalf("status = %v, want 200", got)
	}
	if got := record["trace_id"]; got != "trace-abc" {
		t.Fatalf("trace_id = %v, want trace-abc", got)
	}
}
