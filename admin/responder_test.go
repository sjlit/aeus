package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sjlit/aeus/pkg/errs"
)

// decodeEnvelope parses a responder-emitted body into the wire envelope shape.
func decodeEnvelope(t *testing.T, body []byte) (int, string) {
	t.Helper()
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v body=%s", err, body)
	}
	return env.Code, env.Message
}

func callRespond(t *testing.T, data any) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	res := newResponder()
	res.Respond(rec, httptest.NewRequest(http.MethodGet, "/", nil), data)
	return decodeEnvelope(t, rec.Body.Bytes())
}

// TestRespond_DirectErrorsError covers the happy path: a bare *errs.Error
// (the type-assertion fast path) carries the right code/message.
func TestRespond_DirectErrorsError(t *testing.T) {
	code, msg := callRespond(t, errs.ErrPermissionDenied)
	if code != int(errs.CodePermissionDenied) {
		t.Errorf("code = %d, want %d", code, errs.CodePermissionDenied)
	}
	if msg != errs.ErrPermissionDenied.Message {
		t.Errorf("msg = %q, want %q", msg, errs.ErrPermissionDenied.Message)
	}
}

// TestRespond_WrappedWithFmtErrorf covers the bug the previous implementation
// had: fmt.Errorf("...: %w", e) wraps the *errs.Error in *fmt.wrapError,
// which the old single-step type assertion couldn't unwrap. The frontend
// used to see code=1001 (Invalid) instead of the real 4003 (PermissionDenied),
// routing the response into the wrong branch.
func TestRespond_WrappedWithFmtErrorf(t *testing.T) {
	wrapped := fmt.Errorf("role delete failed: %w", errs.ErrPermissionDenied)
	code, msg := callRespond(t, wrapped)
	if code != int(errs.CodePermissionDenied) {
		t.Errorf("code = %d, want %d (unwrap must reach the inner *errs.Error)",
			code, errs.CodePermissionDenied)
	}
	if msg != errs.ErrPermissionDenied.Message {
		t.Errorf("msg = %q, want %q (inner Message must win over fmt.Errorf prefix)",
			msg, errs.ErrPermissionDenied.Message)
	}
}

// TestRespond_DoubleWrapped covers two fmt.Errorf %w layers deep.
// errors.As must keep walking through both. (We avoid errors.Wrap on top
// of fmt.Errorf here because errs.Wrap intentionally overwrites the
// inner code/message — that path is exercised by TestRespond_PlainError's
// fallback behaviour instead.)
func TestRespond_DoubleWrapped(t *testing.T) {
	inner := errs.New(errs.CodeNotFound, "menu 7 not found")
	wrapped1 := fmt.Errorf("repo.Get failed: %w", inner)
	wrapped2 := fmt.Errorf("service.Delete failed: %w", wrapped1)

	code, msg := callRespond(t, wrapped2)
	if code != int(errs.CodeNotFound) {
		t.Errorf("code = %d, want %d", code, errs.CodeNotFound)
	}
	if msg != "menu 7 not found" {
		t.Errorf("msg = %q, want %q (inner Message must surface through 2 layers)",
			msg, "menu 7 not found")
	}
}

// TestRespond_PlainError keeps the Invalid fallback: when no *errs.Error
// lives anywhere in the chain, the wire must still report a non-OK code.
func TestRespond_PlainError(t *testing.T) {
	plain := errors.New("boom")
	code, msg := callRespond(t, plain)
	if code != int(errs.CodeInvalid) {
		t.Errorf("code = %d, want %d", code, errs.CodeInvalid)
	}
	if msg != "boom" {
		t.Errorf("msg = %q, want %q (raw err.Error() when no *errs.Error found)",
			msg, "boom")
	}
}

// TestRespond_Success locks the success branch: any non-error data goes
// out as code=OK (0) with the payload under `data`.
func TestRespond_Success(t *testing.T) {
	payload := map[string]any{"hello": "world"}
	rec := httptest.NewRecorder()
	res := newResponder()
	res.Respond(rec, httptest.NewRequest(http.MethodGet, "/", nil), payload)

	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want application/json; charset=utf-8", got)
	}

	var env struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	if env.Code != int(errs.CodeOK) {
		t.Errorf("code = %d, want %d", env.Code, errs.CodeOK)
	}
	if env.Message != "" {
		t.Errorf("message = %q, want empty for success", env.Message)
	}
	if env.Data["hello"] != "world" {
		t.Errorf("data.hello = %v, want world", env.Data["hello"])
	}
}
