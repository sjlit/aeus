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

func Wrap(code Code, err error) error {
	return &Error{
		Code:    code,
		Message: err.Error(),
		cause:   err,
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
	case CodeRateLimited, CodeQuotaExceeded:
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
	case CodeRateLimited, CodeQuotaExceeded:
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
