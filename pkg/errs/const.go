package errs

// Code is a business error code. It is a distinct type (not a bare int)
// so that error codes cannot be silently confused with HTTP status codes,
// gRPC codes, or the wire-envelope code field.
type Code int

const (
	CodeOK Code = 0 // success

	// 协议/参数类
	CodeExit              Code = 1000 // normal exit
	CodeInvalid           Code = 1001 // payload invalid
	CodeExists            Code = 1002 // already exists
	CodeUnavailable       Code = 1003 // service unavailable
	CodeIncompatible      Code = 1004 // type incompatible
	CodeInternal          Code = 1005 // internal server error
	CodeTooLarge          Code = 1006 // payload too large
	CodeUnsupportedFormat Code = 1007 // unsupported media/encoding format
	CodeMalformed         Code = 1008 // malformed payload (parse failed)
	CodeMissingField      Code = 1009 // required field missing
	CodeOutOfRange        Code = 1010 // value out of range
	CodeConflict          Code = 1011 // resource conflict (optimistic lock, version mismatch)
	CodeUnprocessable     Code = 1012 // semantically unprocessable (business rule violated)
	CodeNotImplemented    Code = 1013 // feature not implemented

	// 时序类
	CodeTimeout          Code = 2001 // timeout
	CodeExpired          Code = 2002 // expired
	CodeCanceled         Code = 2003 // canceled (client/upstream aborted)
	CodeDeadlineExceeded Code = 2004 // deadline exceeded (context deadline)

	// 限流/配额类
	CodeRateLimited   Code = 3001 // rate limited
	CodeQuotaExceeded Code = 3002 // quota exhausted (daily quota, concurrency cap)

	// 鉴权/资源类
	CodeUnauthorized       Code = 4001 // unauthorized (no credentials)
	CodeTokenExpired       Code = 4002 // token expired
	CodePermissionDenied   Code = 4003 // permission denied
	CodeNotFound           Code = 4004 // not found
	CodeAccessDenied       Code = 4005 // access denied
	CodeTokenInvalid       Code = 4006 // token invalid/forged
	CodePreconditionFailed Code = 4007 // precondition failed (e.g. If-Match mismatch)

	// 网络类
	CodeNetworkUnreachable Code = 5001 // network unreachable
	CodeConnectionRefused  Code = 5002 // connection refused

	// 数据/存储类
	CodeDatabaseError   Code = 6001 // database error
	CodeUniqueViolation Code = 6002 // unique key violation
	CodeDeadlock        Code = 6003 // deadlock (retryable)
	CodeCacheError      Code = 6004 // cache error (Redis/Memcached)

	// 第三方/外部依赖
	CodeUpstreamError Code = 8001 // upstream/external service error
)

var (
	ErrExit              = New(CodeExit, "normal exit")
	ErrInvalid           = New(CodeInvalid, "invalid payload")
	ErrExists            = New(CodeExists, "already exists")
	ErrUnavailable       = New(CodeUnavailable, "service unavailable")
	ErrIncompatible      = New(CodeIncompatible, "type incompatible")
	ErrTooLarge          = New(CodeTooLarge, "payload too large")
	ErrUnsupportedFormat = New(CodeUnsupportedFormat, "unsupported format")
	ErrMalformed         = New(CodeMalformed, "malformed payload")
	ErrMissingField      = New(CodeMissingField, "required field missing")
	ErrOutOfRange        = New(CodeOutOfRange, "value out of range")
	ErrConflict          = New(CodeConflict, "resource conflict")
	ErrUnprocessable     = New(CodeUnprocessable, "semantically unprocessable")
	ErrNotImplemented    = New(CodeNotImplemented, "not implemented")

	ErrTimeout          = New(CodeTimeout, "timeout")
	ErrExpired          = New(CodeExpired, "expired")
	ErrCanceled         = New(CodeCanceled, "canceled")
	ErrDeadlineExceeded = New(CodeDeadlineExceeded, "deadline exceeded")

	ErrRateLimited   = New(CodeRateLimited, "rate limited")
	ErrQuotaExceeded = New(CodeQuotaExceeded, "quota exceeded")

	ErrNotFound           = New(CodeNotFound, "not found")
	ErrAccessDenied       = New(CodeAccessDenied, "access denied")
	ErrPermissionDenied   = New(CodePermissionDenied, "permission denied")
	ErrTokenExpired       = New(CodeTokenExpired, "token expired")
	ErrUnauthorized       = New(CodeUnauthorized, "unauthorized")
	ErrTokenInvalid       = New(CodeTokenInvalid, "token invalid")
	ErrPreconditionFailed = New(CodePreconditionFailed, "precondition failed")

	ErrNetworkUnreachable = New(CodeNetworkUnreachable, "network unreachable")
	ErrConnectionRefused  = New(CodeConnectionRefused, "connection refused")

	ErrDatabaseError   = New(CodeDatabaseError, "database error")
	ErrUniqueViolation = New(CodeUniqueViolation, "unique key violation")
	ErrDeadlock        = New(CodeDeadlock, "deadlock")
	ErrCacheError      = New(CodeCacheError, "cache error")

	ErrUpstreamError = New(CodeUpstreamError, "upstream error")
)
