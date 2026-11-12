package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestError_Error(t *testing.T) {
	e := &Error{Code: 404, Message: "not found"}
	got := e.Error()
	want := "code: 404, message: not found"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestError_Code(t *testing.T) {
	e := &Error{Code: 500}
	if e.Code != 500 {
		t.Errorf("Code = %d, want 500", e.Code)
	}
}

func TestError_Message(t *testing.T) {
	e := &Error{Message: "boom"}
	if e.Message != "boom" {
		t.Errorf("Message = %q, want \"boom\"", e.Message)
	}
}

func TestWrap(t *testing.T) {
	inner := errors.New("db failed")
	got := Wrap(500, inner)
	if got == nil {
		t.Fatal("Wrap returned nil")
	}
	if got.(*Error).Code != 500 {
		t.Errorf("Code = %d, want 500", got.(*Error).Code)
	}
	if got.Error() != "code: 500, message: db failed" {
		t.Errorf("Error() = %q, want \"code: 500, message: db failed\"", got.Error())
	}
}

func TestInternal(t *testing.T) {
	inner := errors.New("db password=secret")
	err := Internal(inner)
	var e *Error
	if !errors.As(err, &e) {
		t.Fatal("Internal error does not unwrap to *Error")
	}
	if e.Code != CodeInternal {
		t.Fatalf("Code = %d, want %d", e.Code, CodeInternal)
	}
	if e.Message != "internal server error" {
		t.Fatalf("Message = %q, want generic message", e.Message)
	}
	if !errors.Is(err, inner) {
		t.Fatal("Internal must retain the cause")
	}
	if Internal(nil) != nil {
		t.Fatal("Internal(nil) must return nil")
	}
}

func TestWrap_InternalCodeDoesNotExposeCause(t *testing.T) {
	inner := errors.New("db password=secret")
	err := Wrap(CodeInternal, inner)
	var e *Error
	if !errors.As(err, &e) {
		t.Fatal("Wrap error does not unwrap to *Error")
	}
	if e.Message != "internal server error" {
		t.Fatalf("Message = %q, want generic message", e.Message)
	}
	if e.Error() == "" || !errors.Is(err, inner) {
		t.Fatal("cause must remain available for logging/errors.Is")
	}
}

func TestWrapPublic(t *testing.T) {
	inner := errors.New("driver detail")
	err := WrapPublic(CodeInvalid, "invalid request", inner)
	var e *Error
	if !errors.As(err, &e) {
		t.Fatal("WrapPublic error does not unwrap to *Error")
	}
	if e.Code != CodeInvalid || e.Message != "invalid request" {
		t.Fatalf("got code=%d message=%q", e.Code, e.Message)
	}
	if !errors.Is(err, inner) {
		t.Fatal("WrapPublic must retain the cause")
	}
}

func TestPublic(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    Code
		message string
	}{
		{"nil", nil, CodeOK, ""},
		{"business", New(CodeNotFound, "user not found"), CodeNotFound, "user not found"},
		{"wrapped business", fmt.Errorf("repo: %w", New(CodePermissionDenied, "no access")), CodePermissionDenied, "no access"},
		{"internal", Internal(errors.New("sql: password=secret")), CodeInternal, "internal server error"},
		{"database", New(CodeDatabaseError, "dial tcp 10.0.0.1:5432"), CodeDatabaseError, "internal server error"},
		{"plain", errors.New("raw secret"), CodeInternal, "internal server error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, message := Public(tc.err)
			if code != tc.want {
				t.Fatalf("code = %d, want %d", code, tc.want)
			}
			if message != tc.message {
				t.Fatalf("message = %q, want %q", message, tc.message)
			}
		})
	}
}

func TestNew(t *testing.T) {
	got := New(0, "success")
	if got.Code != 0 {
		t.Errorf("Code = %d, want 0", got.Code)
	}
	if got.Message != "success" {
		t.Errorf("Message = %q, want \"success\"", got.Message)
	}
}

func TestIs(t *testing.T) {
	inner := ErrNotFound
	wrapped := Wrap(404, inner)
	if !Is(wrapped, ErrNotFound) {
		t.Error("Is(wrapped, ErrNotFound) should be true")
	}
}

func TestErrorConstants(t *testing.T) {
	cases := []struct {
		varName  string
		err      *Error
		wantCode Code
		wantMsg  string
	}{
		{"ErrExit", ErrExit, CodeExit, "normal exit"},
		{"ErrTimeout", ErrTimeout, CodeTimeout, "timeout"},
		{"ErrExpired", ErrExpired, CodeExpired, "expired"},
		{"ErrCanceled", ErrCanceled, CodeCanceled, "canceled"},
		{"ErrDeadlineExceeded", ErrDeadlineExceeded, CodeDeadlineExceeded, "deadline exceeded"},
		{"ErrExists", ErrExists, CodeExists, "already exists"},
		{"ErrInvalid", ErrInvalid, CodeInvalid, "invalid payload"},
		{"ErrMalformed", ErrMalformed, CodeMalformed, "malformed payload"},
		{"ErrMissingField", ErrMissingField, CodeMissingField, "required field missing"},
		{"ErrOutOfRange", ErrOutOfRange, CodeOutOfRange, "value out of range"},
		{"ErrTooLarge", ErrTooLarge, CodeTooLarge, "payload too large"},
		{"ErrUnsupportedFormat", ErrUnsupportedFormat, CodeUnsupportedFormat, "unsupported format"},
		{"ErrConflict", ErrConflict, CodeConflict, "resource conflict"},
		{"ErrUnprocessable", ErrUnprocessable, CodeUnprocessable, "semantically unprocessable"},
		{"ErrNotImplemented", ErrNotImplemented, CodeNotImplemented, "not implemented"},
		{"ErrIncompatible", ErrIncompatible, CodeIncompatible, "type incompatible"},
		{"ErrUnavailable", ErrUnavailable, CodeUnavailable, "service unavailable"},
		{"ErrRateLimited", ErrRateLimited, CodeRateLimited, "rate limited"},
		{"ErrQuotaExceeded", ErrQuotaExceeded, CodeQuotaExceeded, "quota exceeded"},
		{"ErrNotFound", ErrNotFound, CodeNotFound, "not found"},
		{"ErrAccessDenied", ErrAccessDenied, CodeAccessDenied, "access denied"},
		{"ErrPermissionDenied", ErrPermissionDenied, CodePermissionDenied, "permission denied"},
		{"ErrTokenExpired", ErrTokenExpired, CodeTokenExpired, "token expired"},
		{"ErrUnauthorized", ErrUnauthorized, CodeUnauthorized, "unauthorized"},
		{"ErrTokenInvalid", ErrTokenInvalid, CodeTokenInvalid, "token invalid"},
		{"ErrPreconditionFailed", ErrPreconditionFailed, CodePreconditionFailed, "precondition failed"},
		{"ErrNetworkUnreachable", ErrNetworkUnreachable, CodeNetworkUnreachable, "network unreachable"},
		{"ErrConnectionRefused", ErrConnectionRefused, CodeConnectionRefused, "connection refused"},
		{"ErrDatabaseError", ErrDatabaseError, CodeDatabaseError, "database error"},
		{"ErrUniqueViolation", ErrUniqueViolation, CodeUniqueViolation, "unique key violation"},
		{"ErrDeadlock", ErrDeadlock, CodeDeadlock, "deadlock"},
		{"ErrCacheError", ErrCacheError, CodeCacheError, "cache error"},
		{"ErrUpstreamError", ErrUpstreamError, CodeUpstreamError, "upstream error"},
	}
	for _, c := range cases {
		if c.err.Code != c.wantCode {
			t.Errorf("%s.Code = %d, want %d", c.varName, c.err.Code, c.wantCode)
		}
		if c.err.Message != c.wantMsg {
			t.Errorf("%s.Message = %q, want %q", c.varName, c.err.Message, c.wantMsg)
		}
	}
}

func TestError_HTTPStatus(t *testing.T) {
	cases := []struct {
		code Code
		want int
	}{
		{CodeNotFound, 404},
		{CodePermissionDenied, 403},
		{CodeAccessDenied, 403},
		{CodeTokenExpired, 401},
		{CodeUnauthorized, 401},
		{CodeTokenInvalid, 401},
		{CodeInvalid, 400},
		{CodeMalformed, 400},
		{CodeMissingField, 400},
		{CodeOutOfRange, 400},
		{CodeTooLarge, 413},
		{CodeUnsupportedFormat, 415},
		{CodeExists, 409},
		{CodeConflict, 409},
		{CodeUniqueViolation, 409},
		{CodeUnprocessable, 422},
		{CodePreconditionFailed, 412},
		{CodeRateLimited, 429},
		{CodeQuotaExceeded, 429},
		{CodeTimeout, 504},
		{CodeDeadlineExceeded, 504},
		{CodeCanceled, 499},
		{CodeUnavailable, 503},
		{CodeDatabaseError, 503},
		{CodeCacheError, 503},
		{CodeDeadlock, 503},
		{CodeUpstreamError, 503},
		{CodeNotImplemented, 501},
		{CodeInternal, 500},
		{9999, 500}, // unknown code defaults to 500
	}
	for _, c := range cases {
		e := New(c.code, "test")
		if got := e.HTTPStatus(); got != c.want {
			t.Errorf("HTTPStatus() for code %d = %d, want %d", c.code, got, c.want)
		}
	}
}

func TestError_GRPCStatus(t *testing.T) {
	cases := []struct {
		code Code
		want int
	}{
		{CodeNotFound, 5},
		{CodeExists, 6},
		{CodeUniqueViolation, 6},
		{CodePermissionDenied, 7},
		{CodeAccessDenied, 7},
		{CodeTokenExpired, 16},
		{CodeUnauthorized, 16},
		{CodeTokenInvalid, 16},
		{CodeInvalid, 3},
		{CodeMalformed, 3},
		{CodeMissingField, 3},
		{CodeOutOfRange, 3},
		{CodeTooLarge, 3},
		{CodeUnsupportedFormat, 3},
		{CodeUnprocessable, 3},
		{CodeConflict, 10},
		{CodeDeadlock, 10},
		{CodePreconditionFailed, 9},
		{CodeRateLimited, 8},
		{CodeQuotaExceeded, 8},
		{CodeTimeout, 4},
		{CodeDeadlineExceeded, 4},
		{CodeCanceled, 1},
		{CodeUnavailable, 14},
		{CodeDatabaseError, 14},
		{CodeCacheError, 14},
		{CodeUpstreamError, 14},
		{CodeNotImplemented, 12},
		{CodeInternal, 13},
		{9999, 2},
	}
	for _, c := range cases {
		e := New(c.code, "test")
		if got := e.GRPCStatus(); got != c.want {
			t.Errorf("GRPCStatus() for code %d = %v, want %v", c.code, got, c.want)
		}
	}
}

func TestIsCode(t *testing.T) {
	inner := New(CodeNotFound, "not found")
	wrapped := fmt.Errorf("wrapped: %w", inner)

	if !IsCode(wrapped, CodeNotFound) {
		t.Error("IsCode(wrapped, CodeNotFound) should be true")
	}
	if IsCode(wrapped, CodeUnavailable) {
		t.Error("IsCode(wrapped, CodeUnavailable) should be false")
	}
	if IsCode(errors.New("plain"), CodeNotFound) {
		t.Error("IsCode(plain error, CodeNotFound) should be false")
	}
}
