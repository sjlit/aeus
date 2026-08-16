package nats

import (
	"context"
	"encoding/json"

	"github.com/nats-io/nats.go"
	"github.com/sjlit/aeus/infra/broker"
)

type (
	optsContextKey struct{}
	subContextKey  struct{}
)

type (
	Options struct {
		URL     string
		Options []nats.Option
	}

	SubscriberOptions struct {
		Group string
	}
)

type (
	subscription struct {
		s *nats.Subscription
	}

	publication struct {
		err error
		msg *nats.Msg
	}
)

func (s *subscription) Topic() string {
	return s.s.Subject
}
func (s *subscription) Unsubscribe() error {
	return s.s.Unsubscribe()
}

func (e *publication) Topic() string {
	return e.msg.Subject
}
func (e *publication) Message() (*broker.Message, error) {
	msg := &broker.Message{}
	err := json.Unmarshal(e.msg.Data, msg)
	return msg, err
}

func (e *publication) Ack(ctx context.Context) error {
	if e.msg == nil || e.msg.Reply == "" {
		return broker.ErrNotSupported
	}
	return e.msg.Ack(nats.Context(ctx))
}

func NewOptionContext(ctx context.Context, opts *Options) context.Context {
	return context.WithValue(ctx, optsContextKey{}, opts)
}

func NewSubscriberOptionsContext(ctx context.Context, opts *SubscriberOptions) context.Context {
	return context.WithValue(ctx, subContextKey{}, opts)
}

func getOptionFromContext(ctx context.Context) *Options {
	opts, ok := ctx.Value(optsContextKey{}).(*Options)
	if !ok {
		return nil
	}
	return opts
}

func getSubscriberOptionsFromContext(ctx context.Context) *SubscriberOptions {
	opts, ok := ctx.Value(subContextKey{}).(*SubscriberOptions)
	if !ok {
		return nil
	}
	return opts
}

func newSubscription(s *nats.Subscription) *subscription {
	return &subscription{s: s}
}

func newPublication(msg *nats.Msg) *publication {
	return &publication{msg: msg}
}
