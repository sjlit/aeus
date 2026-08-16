package broker

import (
	"context"
	"errors"
)

var (
	ErrNotInitialized     = errors.New("broker not initialized")
	ErrClosed             = errors.New("broker closed")
	ErrAlreadyInitialized = errors.New("broker already initialized")
	ErrNotSupported       = errors.New("operation not supported")
)

// Message is a message send/received from the broker.
type Message struct {
	Header map[string]string `json:"header,omitempty"`
	Body   []byte            `json:"body,omitempty"`
}

type Publication interface {
	Topic() string
	Message() (*Message, error)
	Ack(ctx context.Context) error
}

// Handler processes a single published message.
// The context is per-message: it allows trace context and deadlines to be
// propagated from the broker to the consumer.
type Handler func(ctx context.Context, p Publication) error

type Subscription interface {
	Topic() string
	Unsubscribe() error
}

type Broker interface {
	Init(ctx context.Context) error
	Publish(ctx context.Context, topic string, m *Message) error
	Subscribe(ctx context.Context, topic string, h Handler) (Subscription, error)
	Destroy(ctx context.Context) error
}
