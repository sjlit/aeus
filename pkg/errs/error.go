package errs

import (
	"errors"
	"fmt"
	"net/http"
)

type Error struct {
	Code    Code
	Message string
	cause   error
}

func (e *Error) Error() string {
	return fmt.Sprintf("code: %d, message: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error {
	return e.cause
}

// Wrap annotates err with code and exposes err.Error() as the public
// message. It is kept for backward compatibility; internal errors should
// use Internal or WrapPublic so driver/DB details never reach clients.
func Wrap(code Code, err error) error {
	message := err.Error()
	if isInternalCode(code) {
		// Wrapping an internal failure must not turn its cause into a
		// client-facing message by accident.
		message = "internal server error"
	}
	return &Error{
		Code:    code,
		Message: message,
		cause:   err,
	}
}

// Internal wraps an implementation error with CodeInternal and a fixed
// public message. The original error is retained for errors.Is/As and
// server-side logging but is never serialized to a client.
func Internal(err error) error {
	if err == nil {
		return nil
	}
	return &Error{
		Code:    CodeInternal,
		Message: "internal server error",
		cause:   err,
	}
}

// WrapPublic builds an Error with an explicit client-safe message and a
// separate internal cause.
func WrapPublic(code Code, message string, cause error) error {
	return &Error{
		Code:    code,
		Message: message,
		cause:   cause,
	}
}

// Public returns the code and client-safe message for err. Business
// errors keep their message; internal/infrastructure failures collapse
// to a generic message so their cause never reaches a response. Errors
// outside the errs package are always treated as internal.
func Public(err error) (Code, string) {
	if err == nil {
		return CodeOK, ""
	}
	var e *Error
	if errors.As(err, &e) {
		if isInternalCode(e.Code) {
			return e.Code, "internal server error"
		}
		return e.Code, e.Message
	}
	return CodeInternal, "internal server error"
}

func isInternalCode(code Code) bool {
	switch code {
	case CodeInternal,
		CodeDatabaseError,
		CodeCacheError,
		CodeDeadlock,
		CodeUpstreamError:
		return true
	default:
		return false
	}
}

func Is(err, target error) bool {
	return errors.Is(err, target)
}

// Newf builds an Error with a formatted message.
func Newf(code Code, message string, args ...any) error {
	return &Error{
		Code:    code,
		Message: fmt.Sprintf(message, args...),
	}
}

func New(code Code, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

func (e *Error) HTTPStatus() int {
	switch e.Code {
	case CodeNotFound:
		return http.StatusNotFound
	case CodePermissionDenied, CodeAccessDenied:
		return http.StatusForbidden
	case CodeTokenExpired, CodeUnauthorized, CodeTokenInvalid:
		return http.StatusUnauthorized
	case CodeInvalid, CodeMalformed, CodeMissingField, CodeOutOfRange:
		return http.StatusBadRequest
	case CodeTooLarge:
		return http.StatusRequestEntityTooLarge
	case CodeUnsupportedFormat:
		return http.StatusUnsupportedMediaType
	case CodeExists, CodeConflict, CodeUniqueViolation:
		return http.StatusConflict
	case CodeUnprocessable:
		return http.StatusUnprocessableEntity
	case CodePreconditionFailed:
		return http.StatusPreconditionFailed
	case CodeRateLimited, CodeQuotaExceeded, CodeTooManyAttempts:
		return http.StatusTooManyRequests
	case CodeTimeout, CodeDeadlineExceeded:
		return http.StatusGatewayTimeout
	case CodeCanceled:
		return 499 // Client Closed Request (nginx convention)
	case CodeUnavailable, CodeDatabaseError, CodeCacheError, CodeDeadlock, CodeUpstreamError:
		return http.StatusServiceUnavailable
	case CodeNotImplemented:
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

const (
	grpcCanceled           = 1  // codes.CodeCanceled
	grpcUnknown            = 2  // codes.Unknown
	grpcInvalidArgument    = 3  // codes.InvalidArgument
	grpcDeadlineExceeded   = 4  // codes.CodeDeadlineExceeded
	grpcNotFound           = 5  // codes.CodeNotFound
	grpcAlreadyExists      = 6  // codes.AlreadyExists
	grpcPermissionDenied   = 7  // codes.CodePermissionDenied
	grpcResourceExhausted  = 8  // codes.ResourceExhausted
	grpcFailedPrecondition = 9  // codes.FailedPrecondition
	grpcAborted            = 10 // codes.Aborted
	grpcUnimplemented      = 12 // codes.Unimplemented
	grpcInternal           = 13 // codes.CodeInternal
	grpcUnavailable        = 14 // codes.CodeUnavailable
	grpcUnauthenticated    = 16 // codes.Unauthenticated
)

func (e *Error) GRPCStatus() int {
	switch e.Code {
	case CodeNotFound:
		return grpcNotFound
	case CodeExists, CodeUniqueViolation:
		return grpcAlreadyExists
	case CodePermissionDenied, CodeAccessDenied:
		return grpcPermissionDenied
	case CodeInvalid, CodeMalformed, CodeMissingField, CodeOutOfRange, CodeTooLarge, CodeUnsupportedFormat, CodeUnprocessable:
		return grpcInvalidArgument
	case CodeConflict, CodeDeadlock:
		return grpcAborted
	case CodePreconditionFailed:
		return grpcFailedPrecondition
	case CodeRateLimited, CodeQuotaExceeded, CodeTooManyAttempts:
		return grpcResourceExhausted
	case CodeTimeout, CodeDeadlineExceeded:
		return grpcDeadlineExceeded
	case CodeCanceled:
		return grpcCanceled
	case CodeUnavailable, CodeDatabaseError, CodeCacheError, CodeUpstreamError:
		return grpcUnavailable
	case CodeNotImplemented:
		return grpcUnimplemented
	case CodeTokenExpired, CodeUnauthorized, CodeTokenInvalid:
		return grpcUnauthenticated
	case CodeInternal:
		return grpcInternal
	default:
		return grpcUnknown
	}
}

func IsCode(err error, code Code) bool {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code == code
	}
	return false
}
