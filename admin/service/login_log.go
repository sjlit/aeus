package service

import (
	"context"
)

// LoginLogInfo is the data shape passed to a LoginLogFunc installed via
// WithLoginLogger. On failed attempts UID/TenantID are deliberately
// empty to avoid leaking whether a given username exists; the audit row
// only needs IP/UserAgent/Username to support rate-limit decisions.
type LoginLogInfo struct {
	Success     bool
	UID         string
	TenantID    string
	Username    string
	IP          string
	UserAgent   string
	AccessToken string
}

// LoginLogFunc receives every login attempt — successful or failed —
// after AuthService.Login has finished signing tokens (success) or
// returned an error (failure). It runs synchronously on the login path;
// implementations should not panic; panics propagate to the caller.
type LoginLogFunc func(ctx context.Context, info LoginLogInfo)

// WithLoginLogger enables structured login auditing. When fn is nil
// (the default), no recorder is called — matching the pre-Logger
// behaviour exactly.
func WithLoginLogger(fn LoginLogFunc) AuthServiceOption {
	return func(o *AuthServiceOptions) {
		o.LoginLogFunc = fn
	}
}
