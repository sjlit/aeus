// Package response owns the wire envelope shared by framework transports
// and the admin module.
//
// The contract is intentionally small: every response body is
// {code, message, data}, where code=0 means success. Business errors are
// written with HTTP 200 by the admin responder; transport-level errors
// (authentication, rate limiting, not-found, panic) may use standard HTTP
// status codes while keeping the same envelope shape. Keeping the struct
// and writer in one place stops the two copies from drifting apart.
package response

import (
	"encoding/json"
	"net/http"
)

// Envelope is the JSON/XML/YAML shape every aeus response uses.
type Envelope struct {
	Code    int    `json:"code" xml:"code" yaml:"code"`
	Message string `json:"message" xml:"message" yaml:"message"`
	Data    any    `json:"data,omitempty" xml:"data,omitempty" yaml:"data,omitempty"`
}

// SetCode implements the legacy transport/http Response interface.
func (e *Envelope) SetCode(code int) { e.Code = code }

// SetMessage implements the legacy transport/http Response interface.
func (e *Envelope) SetMessage(message string) { e.Message = message }

// SetData implements the legacy transport/http Response interface.
func (e *Envelope) SetData(data any) { e.Data = data }

// New builds an envelope. It is the package-level equivalent of the old
// transport/http newResponse helper.
func New(code int, message string, data any) *Envelope {
	return &Envelope{Code: code, Message: message, Data: data}
}

// Write serializes env and writes it with the supplied HTTP status.
//
// A status of 0 falls back to 200. A marshal failure (unsupported Data
// type, cyclic value, ...) degrades to a minimal invalid-request envelope
// instead of dropping the response bytes entirely.
func Write(w http.ResponseWriter, status int, env *Envelope) error {
	if status == 0 {
		status = http.StatusOK
	}
	if env == nil {
		env = &Envelope{Code: 1005, Message: "empty response"}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	body, err := json.Marshal(env)
	if err != nil {
		fallback, _ := json.Marshal(&Envelope{
			Code:    1001,
			Message: "marshal failure: " + err.Error(),
		})
		_, writeErr := w.Write(fallback)
		return writeErr
	}
	_, err = w.Write(body)
	return err
}
