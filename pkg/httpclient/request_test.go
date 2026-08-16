package httpclient

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetectContentTypeNil(t *testing.T) {
	r := &Request{}
	ct := r.detectContentType(nil)
	if ct != "" {
		t.Errorf("detectContentType(nil) = %q, want empty string", ct)
	}
}

func TestDetectContentTypeNilPointer(t *testing.T) {
	r := &Request{}
	var p *struct{ Name string }
	// This should not panic
	_ = r.detectContentType(p)
}

func TestRequestDoDoesNotOverrideBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"received":%q}`, string(body))
	}))
	defer ts.Close()

	client := New()
	var result map[string]string
	req := client.Post(ts.URL).
		SetBody(`{"custom":"payload"}`).
		AddFormData("foo", "bar")

	err := req.Response(&result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After fix, the body should remain the JSON payload, not form data
	if result["received"] != `{"custom":"payload"}` {
		t.Errorf("received = %q, want JSON payload; formData may have overridden body", result["received"])
	}
}
