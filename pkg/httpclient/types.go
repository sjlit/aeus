package httpclient

import (
	"crypto/tls"
	"maps"
	"net/http"
)

type (
	options struct {
		method         string
		header         map[string]string
		params         map[string]string
		body           any
		browserHeaders bool
		client         *http.Client
	}
	Option func(o *options)
)

func WithMethod(s string) Option {
	return func(o *options) {
		o.method = s
	}
}

// WithBrowserHeaders makes the request look browser-originated by setting a
// browser User-Agent and Referer header when they are absent.
func WithBrowserHeaders() Option {
	return func(o *options) {
		o.browserHeaders = true
	}
}

func WithClient(c *http.Client) Option {
	return func(o *options) {
		o.client = c
	}
}

func WithHeader(h map[string]string) Option {
	return func(o *options) {
		if o.header == nil {
			o.header = make(map[string]string)
		}
		maps.Copy(o.header, h)
	}
}

func WithParams(h map[string]string) Option {
	return func(o *options) {
		if o.params == nil {
			o.params = make(map[string]string)
		}
		maps.Copy(o.params, h)
	}
}

func WithBody(v any) Option {
	return func(o *options) {
		o.body = v
	}
}

func WithInsecure() Option {
	return func(o *options) {
		transport, ok := o.client.Transport.(*http.Transport)
		if !ok {
			return
		}
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.InsecureSkipVerify = true
	}
}

func newOptions() *options {
	return &options{
		client: DefaultClient,
		method: http.MethodGet,
	}
}
