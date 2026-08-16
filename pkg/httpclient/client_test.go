package httpclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultClientDoesNotSkipTLSVerify(t *testing.T) {
	transport, ok := DefaultClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("DefaultClient.Transport is not *http.Transport")
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("DefaultClient should not skip TLS verification")
	}
}

func TestWithInsecureEnablesSkipVerify(t *testing.T) {
	opts := newOptions()
	if opts.client == nil {
		t.Fatal("opts.client is nil")
	}
	transport, ok := opts.client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("opts.client.Transport is not *http.Transport")
	}
	if transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("newOptions client should not skip TLS by default")
	}

	WithInsecure()(opts)
	transport, ok = opts.client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("after WithInsecure, Transport is not *http.Transport")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("WithInsecure should enable skip verify")
	}
}

func TestInterceptorOrder(t *testing.T) {
	var order []string
	client := New()
	client.BeforeRequest(func(c *http.Client, req *http.Request) error {
		order = append(order, "first")
		return nil
	})
	client.BeforeRequest(func(c *http.Client, req *http.Request) error {
		order = append(order, "second")
		return nil
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, err := client.Get(ts.URL).Do()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("interceptor order = %v, want [first second]", order)
	}
}

func TestClientConcurrentAccess(t *testing.T) {
	client := New()

	go func() {
		client.SetBaseURL("http://example.com")
	}()
	go func() {
		client.BeforeRequest(func(c *http.Client, req *http.Request) error { return nil })
	}()
	go func() {
		_ = client.Get("/test")
	}()
}
