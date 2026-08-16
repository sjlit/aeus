package http

import (
	"testing"
)

func TestNewResponse(t *testing.T) {
	r := newResponse(200, "ok", map[string]any{"id": 1})
	if r == nil {
		t.Fatal("newResponse returned nil")
	}
	resp, ok := r.(*response)
	if !ok {
		t.Fatal("newResponse should return *response")
	}
	if resp.Code != 200 {
		t.Errorf("Code = %d, want 200", resp.Code)
	}
	if resp.Message != "ok" {
		t.Errorf("Message = %q, want \"ok\"", resp.Message)
	}
	if resp.Data == nil {
		t.Error("Data should not be nil")
	}
}

func TestResponse_SetCode(t *testing.T) {
	r := &response{}
	r.SetCode(404)
	if r.Code != 404 {
		t.Errorf("Code = %d, want 404", r.Code)
	}
}

func TestResponse_SetMessage(t *testing.T) {
	r := &response{}
	r.SetMessage("not found")
	if r.Message != "not found" {
		t.Errorf("Message = %q, want \"not found\"", r.Message)
	}
}

func TestResponse_SetData(t *testing.T) {
	r := &response{}
	data := []string{"a", "b"}
	r.SetData(data)
	if r.Data == nil {
		t.Error("Data should not be nil")
	}
}

func TestResponse_Interface(t *testing.T) {
	var _ Response = newResponse(0, "", nil)
}
