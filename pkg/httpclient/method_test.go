package httpclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoAccepts201(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintln(w, `{"id":123}`)
	}))
	defer ts.Close()

	var result map[string]int
	err := Do(context.Background(), ts.URL, &result)
	if err != nil {
		t.Fatalf("Do should accept 201, got error: %v", err)
	}
	if result["id"] != 123 {
		t.Errorf("result = %+v, want id=123", result)
	}
}

func TestDoAccepts204(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	err := Do(context.Background(), ts.URL, nil)
	if err != nil {
		t.Fatalf("Do should accept 204, got error: %v", err)
	}
}
