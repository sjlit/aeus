package httpclient

import (
	"net/http"
	"net/http/cookiejar"
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

// TestNew_ClientsDoNotShareUnderlyingHTTPClient verifies that two
// clients built by New() are independent: their *http.Client fields
// must not alias the same package-level DefaultClient, otherwise
// SetTransport / SetClient / SetCookieJar on one client will mutate
// state visible to every other client (and to DefaultClient itself).
func TestNew_ClientsDoNotShareUnderlyingHTTPClient(t *testing.T) {
	a := New()
	b := New()
	if a.client == b.client {
		t.Fatal("New() returned two clients sharing the same *http.Client; expected independent instances")
	}
	if a.client == DefaultClient {
		t.Fatal("New() returned a client whose *http.Client is the package-level DefaultClient")
	}
	if b.client == DefaultClient {
		t.Fatal("New() returned a client whose *http.Client is the package-level DefaultClient")
	}
}

// TestNew_ClientsHaveIndependentCookieJars verifies that the Jar
// installed by New() does not leak across instances. The previous
// implementation wrote each new client's cookieJar into the shared
// DefaultClient.Jar, so the second New() call would silently
// overwrite the first client's jar on the shared field.
func TestNew_ClientsHaveIndependentCookieJars(t *testing.T) {
	a := New()
	b := New()
	if a.client.Jar == nil || b.client.Jar == nil {
		t.Fatal("New() must install a non-nil cookie jar on each client")
	}
	if a.client.Jar == b.client.Jar {
		t.Fatal("two New() clients share the same cookieJar; expected independent jars")
	}
	// Sanity: a.cookieJar (the Client field) must point at the same
	// jar as a.client.Jar — otherwise the client has two jars.
	if a.cookieJar != a.client.Jar {
		t.Fatal("Client.cookieJar and Client.client.Jar are out of sync")
	}
	if b.cookieJar != b.client.Jar {
		t.Fatal("Client.cookieJar and Client.client.Jar are out of sync")
	}
}

// TestSetTransport_OnlyAffectsOwnClient verifies that calling
// SetTransport on one client does not change the Transport seen by
// any other client. The previous implementation mutated the shared
// DefaultClient.Transport, leaking the change to every other client.
func TestSetTransport_OnlyAffectsOwnClient(t *testing.T) {
	a := New()
	b := New()
	custom := &http.Transport{}
	a.SetTransport(custom)
	if b.client.Transport == custom {
		t.Fatal("SetTransport on client A leaked to client B")
	}
	if DefaultClient.Transport == custom {
		t.Fatal("SetTransport on client A leaked to DefaultClient")
	}
}

// TestSetClient_OnlyAffectsOwnClient verifies that calling SetClient
// on one client does not change the *http.Client seen by another.
// SetClient assigns client.client directly, so the only way this can
// leak is if the assignment overwrote a shared reference — which is
// not the case in a fixed implementation, but the test pins the
// contract so a future regression is caught.
func TestSetClient_OnlyAffectsOwnClient(t *testing.T) {
	a := New()
	b := New()
	originalB := b.client
	replacement := &http.Client{Transport: &http.Transport{}}
	a.SetClient(replacement)
	if b.client != originalB {
		t.Fatal("SetClient on client A changed the *http.Client of client B")
	}
}

// TestNew_DoesNotMutateDefaultClient is a smoke test that captures
// the surface symptom of the original bug: after several New() calls
// the package-level DefaultClient must still expose its original
// jar/transport, not whatever the latest client happened to install.
func TestNew_DoesNotMutateDefaultClient(t *testing.T) {
	// Snapshot DefaultClient's transport pointer before any New() calls.
	beforeTransport := DefaultClient.Transport
	beforeJar := DefaultClient.Jar

	// Spin up several clients — any of them would, under the old code,
	// overwrite DefaultClient.Jar with its own.
	for range 5 {
		_ = New()
	}

	if DefaultClient.Transport != beforeTransport {
		t.Fatal("DefaultClient.Transport was mutated by New()")
	}
	if DefaultClient.Jar != beforeJar {
		t.Fatal("DefaultClient.Jar was mutated by New()")
	}
}

// _ keeps the cookiejar import live for future tests that exercise
// cookie state across clients.
var _ = cookiejar.New
