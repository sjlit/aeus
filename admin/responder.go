package admin

import (
	"errors"
	"net/http"

	"github.com/sjlit/aeus/infra/logger"
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
// writer. The logger is used to retain details that are deliberately
// withheld from the client.
func newResponder(lg logger.Logger) rest.Responder {
	if lg == nil {
		lg = logger.Default()
	}
	return responderFunc(func(w http.ResponseWriter, r *http.Request, data any) {
		respond(lg, w, r, data)
	})
}

// respond turns data into an envelope and writes it via writeEnvelope.
// When data is an error, the business code is recovered from the
// deepest *errs.Error in its Unwrap chain. Internal/infrastructure
// failures and plain errors collapse to CodeInternal with a generic
// message; their cause is logged instead of serialized.
func respond(lg logger.Logger, w http.ResponseWriter, r *http.Request, data any) {
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
		// toast instead of "jump to login").
		var e *errs.Error
		if errors.As(err, &e) {
			code, message = errs.Public(err)
		} else {
			code = errs.CodeInternal
			message = "internal server error"
			if lg != nil {
				lg.Errorf(r.Context(), "admin responder: unhandled error: %v", err)
			}
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
