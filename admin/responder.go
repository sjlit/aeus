package admin

import (
	"errors"
	"net/http"

	"github.com/sjlit/aeus/pkg/errs"
	responsepkg "github.com/sjlit/aeus/pkg/response"
	"github.com/sjlit/rest/v3"
)

// responderFunc adapts respond to rest.Responder, so newOptions can
// hand the stateless respond function straight to rest/v3 without a
// carrier struct.
type responderFunc func(w http.ResponseWriter, r *http.Request, data any)

// Respond implements rest.Responder by delegating to f.
func (f responderFunc) Respond(w http.ResponseWriter, r *http.Request, data any) {
	f(w, r, data)
}

// newResponder returns the default rest.Responder: the admin envelope
// writer.
func newResponder() rest.Responder {
	return responderFunc(respond)
}

// respond turns data into an envelope and writes it via writeEnvelope.
// When data is an error, the business code is recovered from the
// deepest *errs.Error in its Unwrap chain; anything else is a
// success with code 0 and data as the payload.
func respond(w http.ResponseWriter, r *http.Request, data any) {
	code := errs.CodeOK
	message := ""
	payload := data
	if err, ok := data.(error); ok {
		// Walk the Unwrap chain (fmt.Errorf("...: %w", ...), etc.) to
		// recover the original *errs.Error.  A direct type assertion
		// `err.(*errs.Error)` only matches the outermost layer, so
		// wrapped errors used to fall back to errs.CodeInvalid (=1001)
		// and lose the real business code — the frontend then misroutes
		// the response (e.g. 4003 PermissionDenied → 1001 Invalid →
		// toast instead of "jump to login").  errors.As is the same
		// pattern pkg/errs itself uses inside IsCode
		// (pkg/errs/error.go:107-113).  Plain errors (no
		// *errs.Error anywhere in the chain) keep the Invalid
		// fallback so the wire never reports a misleading 0/OK.
		code = errs.CodeInvalid
		var e *errs.Error
		if errors.As(err, &e) {
			code = e.Code
			message = e.Message
		}
		if message == "" {
			message = err.Error()
		}
		payload = nil
	}
	writeEnvelope(w, int(code), message, payload)
}

// writeEnvelope serializes a single {code, message, data} body and
// writes it with HTTP 200. admin's convention keeps the HTTP status at
// 200 across all success and business-error paths; the envelope's code
// field carries the business outcome.
//
// The writer itself lives in pkg/response so transport-level errors
// (JWT, rate limiting, not-found) and admin business responses share one
// envelope implementation and cannot drift.
func writeEnvelope(w http.ResponseWriter, code int, message string, data any) {
	_ = responsepkg.Write(w, http.StatusOK, responsepkg.New(code, message, data))
}
