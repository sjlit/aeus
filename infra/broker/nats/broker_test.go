package nats

import (
	"context"
	"errors"
	"testing"
	"time"

	nats "github.com/nats-io/nats.go"
	"github.com/sjlit/aeus/infra/broker"
)

func skipIfNoNATS(t *testing.T) string {
	t.Helper()
	nc, err := nats.Connect(nats.DefaultURL)
	if err != nil {
		t.Skipf("no NATS server available at %s: %v", nats.DefaultURL, err)
	}
	nc.Close()
	return nats.DefaultURL
}

func TestNewBroker_ReturnsNonNil(t *testing.T) {
	b := New()
	if b == nil {
		t.Fatal("New() returned nil")
	}
}

func TestPublish_WithoutInit_ReturnsErrNotInitialized(t *testing.T) {
	b := New()
	err := b.Publish(context.Background(), "test", &broker.Message{Body: []byte("hello")})
	if err != broker.ErrNotInitialized {
		t.Fatalf("expected ErrNotInitialized, got %v", err)
	}
}

func TestSubscribe_WithoutInit_ReturnsErrNotInitialized(t *testing.T) {
	b := New()
	_, err := b.Subscribe(context.Background(), "test", func(_ context.Context, p broker.Publication) error { return nil })
	if err != broker.ErrNotInitialized {
		t.Fatalf("expected ErrNotInitialized, got %v", err)
	}
}

func TestDestroy_WithoutInit_NoPanic(t *testing.T) {
	b := New()
	if err := b.Destroy(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInit_Twice_ReturnsErrAlreadyInitialized(t *testing.T) {
	url := skipIfNoNATS(t)
	b := New()
	ctx := NewOptionContext(context.Background(), &Options{URL: url})
	if err := b.Init(ctx); err != nil {
		t.Fatalf("first init failed: %v", err)
	}
	defer b.Destroy(context.Background())

	if err := b.Init(ctx); err != broker.ErrAlreadyInitialized {
		t.Fatalf("expected ErrAlreadyInitialized, got %v", err)
	}
}

func TestPublish_AfterDestroy_ReturnsErrClosed(t *testing.T) {
	url := skipIfNoNATS(t)
	b := New()
	ctx := NewOptionContext(context.Background(), &Options{URL: url})
	if err := b.Init(ctx); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := b.Destroy(context.Background()); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	err := b.Publish(context.Background(), "test", &broker.Message{Body: []byte("hello")})
	if err != broker.ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestSubscribe_AfterDestroy_ReturnsErrClosed(t *testing.T) {
	url := skipIfNoNATS(t)
	b := New()
	ctx := NewOptionContext(context.Background(), &Options{URL: url})
	if err := b.Init(ctx); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if err := b.Destroy(context.Background()); err != nil {
		t.Fatalf("destroy failed: %v", err)
	}

	_, err := b.Subscribe(context.Background(), "test", func(_ context.Context, p broker.Publication) error { return nil })
	if err != broker.ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestPublicationAck_NonJetStream_ReturnsErrNotSupported(t *testing.T) {
	url := skipIfNoNATS(t)
	b := New()
	ctx := NewOptionContext(context.Background(), &Options{URL: url})
	if err := b.Init(ctx); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	defer b.Destroy(context.Background())

	done := make(chan broker.Publication, 1)
	_, err := b.Subscribe(ctx, "test.ack", func(_ context.Context, p broker.Publication) error {
		done <- p
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	if err := b.Publish(ctx, "test.ack", &broker.Message{Body: []byte("hi")}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	pub := <-done
	ackErr := pub.Ack(context.Background())
	if ackErr != broker.ErrNotSupported {
		t.Fatalf("expected ErrNotSupported, got %v", ackErr)
	}
}

func TestSubscribe_HandlerError_IsCaptured(t *testing.T) {
	url := skipIfNoNATS(t)
	b := New()
	ctx := NewOptionContext(context.Background(), &Options{URL: url})
	if err := b.Init(ctx); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	defer b.Destroy(context.Background())

	type handlerCall struct {
		ctx context.Context
		err error
	}
	calls := make(chan handlerCall, 1)
	_, err := b.Subscribe(ctx, "test.err", func(hctx context.Context, p broker.Publication) error {
		err := errors.New("handler failed")
		calls <- handlerCall{ctx: hctx, err: err}
		return err
	})
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	if err := b.Publish(ctx, "test.err", &broker.Message{Body: []byte("hi")}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case call := <-calls:
		if call.err == nil {
			t.Fatal("expected handler error, got nil")
		}
		if call.ctx == nil {
			t.Fatal("handler must receive a non-nil context")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for handler")
	}
}
