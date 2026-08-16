package logger

import (
	"context"
	"testing"
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
