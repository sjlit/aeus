package httpclient

import (
	"testing"
)

func TestEncodeBodyStringContentType(t *testing.T) {
	_, ct, err := encodeBody("foo=bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct != "application/x-www-form-urlencoded" {
		t.Errorf("string body content-type = %q, want %q", ct, "application/x-www-form-urlencoded")
	}
}

func TestEncodeBodyBytesContentType(t *testing.T) {
	_, ct, err := encodeBody([]byte("foo=bar"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct != "application/x-www-form-urlencoded" {
		t.Errorf("[]byte body content-type = %q, want %q", ct, "application/x-www-form-urlencoded")
	}
}
