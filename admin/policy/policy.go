// Package policy defines the neutral route-policy shape emitted by
// protoc-gen-go-aeus for business RPCs. It lives in the admin module so
// generated projects can register the policies without importing the
// admin server implementation.
package policy

import "strings"

// Policy is the authorization policy of one generated HTTP route.
//
// Path is the gin route template (e.g. "/shop/orders/:id"), not the
// concrete request path. Method is uppercase. Button is an optional
// front-end-only permission code.
type Policy struct {
	Method      string
	Path        string
	Button      string
	Group       string
	Description string
	Public      bool
}

// Permission returns the API permission code used by sys_permissions
// and admin.NewPermissionChecker.
func (p Policy) Permission() string {
	if p.Method == "" || p.Path == "" {
		return ""
	}
	return strings.ToUpper(p.Method) + " " + p.Path
}
