package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sjlit/aeus/pkg/errs"
	"github.com/sjlit/rest/v3"
)

// envelope is the wire shape admin emits on every endpoint: a flat
// {code, message, data} object.  The business outcome travels in code
// (0 = OK) while the HTTP status stays 200 on all business paths; see
// writeEnvelope.
type envelope struct {
	Code    int    `json:"code" xml:"code" yaml:"code"`
	Message string `json:"message" xml:"message" yaml:"message"`
	Data    any    `json:"data,omitempty" xml:"data,omitempty" yaml:"data,omitempty"`
}

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
// writes it with HTTP 200.  admin's convention keeps the HTTP status
// at 200 across all success and business-error paths; the envelope's
// code field carries the business outcome.
func writeEnvelope(w http.ResponseWriter, code int, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	// json.Marshal + w.Write is cheaper than json.NewEncoder+Encode on
	// the login hot path: NewEncoder allocates an *Encoder plus an
	// internal 4 KiB bufio.Writer and flushes per call, which dwarfs
	// the work of marshaling a 3-field envelope.
	body, err := json.Marshal(envelope{Code: code, Message: message, Data: data})
	if err != nil {
		// Marshal only fails if data holds an unmarshalable value
		// (channels, funcs, cyclic data).  Fall back to a minimal error
		// envelope instead of dropping the response.
		body, _ = json.Marshal(envelope{
			Code:    int(errs.CodeInvalid),
			Message: "marshal failure: " + err.Error(),
		})
	}
	_, _ = w.Write(body)
}
