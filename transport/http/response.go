package http

import (
	responsepkg "github.com/sjlit/aeus/pkg/response"
)

// Response is the legacy setter-based response interface. It is kept for
// callers that configure a response field-by-field; the concrete type is
// now the shared response.Envelope so the admin and transport envelopes
// cannot drift.
type Response interface {
	SetCode(int)
	SetMessage(string)
	SetData(any)
}

// response aliases the shared envelope. The alias keeps the existing
// unexported name used by tests and newResponse while sharing the wire
// shape with admin.
type response = responsepkg.Envelope

func newResponse(code int, message string, data any) Response {
	return responsepkg.New(code, message, data)
}
