package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sjlit/aeus/pkg/errs"
)

type captureMetrics struct {
	statuses []string
}

func (m *captureMetrics) RecordRequestDuration(context.Context, string, string, string, time.Duration) {
}

func (m *captureMetrics) RecordRequestTotal(_ context.Context, _, _, status string) {
	m.statuses = append(m.statuses, status)
}

func (m *captureMetrics) RecordRegistryHeartbeat(context.Context, string) {}

// TestServer_RecoveryInterceptor locks the panic contract: a panicking
// handler must produce the standard envelope with HTTP 500 and the
// internal-error business code instead of escaping to net/http.
func TestServer_RecoveryInterceptor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New()
	s.engine.GET("/panic", func(*gin.Context) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	s.engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v body=%s", err, rec.Body.String())
	}
	if env.Code != int(errs.CodeInternal) {
		t.Fatalf("code = %d, want %d", env.Code, errs.CodeInternal)
	}
	if env.Message != "internal server error" {
		t.Fatalf("message = %q, want internal server error", env.Message)
	}
}

// TestServer_RecoveryRecordsMetrics pins the ordering requirement: the
// inner recovery must run before requestInterceptor returns, otherwise a
// panicking handler is missing from request metrics.
func TestServer_RecoveryRecordsMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New()
	recorder := &captureMetrics{}
	s.SetMetrics(recorder)
	s.engine.GET("/panic-metrics", func(*gin.Context) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	s.engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic-metrics", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if len(recorder.statuses) != 1 || recorder.statuses[0] != "500" {
		t.Fatalf("metrics statuses = %v, want [500]", recorder.statuses)
	}
}

// TestServer_GenericErrorDoesNotLeakDetail ensures the transport-level
// 500 envelope is stable and does not echo the underlying error text.
func TestServer_GenericErrorDoesNotLeakDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := New()
	s.engine.GET("/fail-secret", func(c *gin.Context) {
		_ = c.Error(errors.New("db password=super-secret"))
	})

	rec := httptest.NewRecorder()
	s.engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/fail-secret", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "super-secret") {
		t.Fatalf("response leaked internal error: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Fatalf("response missing generic message: %s", rec.Body.String())
	}
}
