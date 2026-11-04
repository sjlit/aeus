package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWrite_EnvelopeAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := Write(rec, http.StatusForbidden, New(4003, "permission denied", nil)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
	var env Envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Code != 4003 || env.Message != "permission denied" {
		t.Fatalf("envelope = %+v", env)
	}
}

func TestWrite_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := Write(rec, 0, New(0, "", map[string]any{"ok": true})); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
