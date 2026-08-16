package nats

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	nats "github.com/nats-io/nats.go"
	"github.com/sjlit/aeus/infra/broker"
)

type Broker struct {
	mu     sync.RWMutex
	conn   *nats.Conn
	closed bool
}

func (b *Broker) Init(ctx context.Context) (err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.conn != nil {
		return broker.ErrAlreadyInitialized
	}

	opts := getOptionFromContext(ctx)
	if opts == nil {
		return io.ErrClosedPipe
	}
	if opts.URL == "" {
		opts.URL = nats.DefaultURL
	}
	b.conn, err = nats.Connect(opts.URL, opts.Options...)
	return
}

func (b *Broker) Publish(ctx context.Context, topic string, m *broker.Message) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return broker.ErrClosed
	}
	if b.conn == nil {
		return broker.ErrNotInitialized
	}

	buf, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return b.conn.Publish(topic, buf)
}

func (b *Broker) Subscribe(ctx context.Context, topic string, h broker.Handler) (sb broker.Subscription, err error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return nil, broker.ErrClosed
	}
	if b.conn == nil {
		return nil, broker.ErrNotInitialized
	}

	opts := getSubscriberOptionsFromContext(ctx)
	handleFunc := func(msg *nats.Msg) {
		pub := newPublication(msg)
		// NATS callbacks carry no per-message context; use a fresh one.
		// Brokers that propagate trace context can extract it from msg headers here.
		if herr := h(context.Background(), pub); herr != nil {
			// Attempt negative acknowledgement for JetStream messages.
			// Non-JetStream subscriptions will return an error here; ignore it.
			_ = msg.Nak()
		}
	}

	var sp *nats.Subscription
	if opts != nil && opts.Group != "" {
		sp, err = b.conn.QueueSubscribe(topic, opts.Group, handleFunc)
	} else {
		sp, err = b.conn.Subscribe(topic, handleFunc)
	}
	if err != nil {
		return nil, err
	}
	return newSubscription(sp), nil
}

func (b *Broker) Destroy(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true
	if b.conn != nil {
		b.conn.Close()
	}
	return nil
}

func New() *Broker {
	return &Broker{}
}
