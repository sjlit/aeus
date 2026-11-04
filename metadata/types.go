package metadata

const (
	RequestID   = "X-AEUS-Request-ID"
	RequestPath = "X-AEUS-Request-Path"
	// RequestRoute is the matched route template (gin's FullPath). It is
	// empty for NoRoute/static requests, which is the signal the
	// permission checker uses to treat them as infrastructure instead of
	// a fail-closed business API.
	RequestRoute    = "X-AEUS-Request-Route"
	RequestMethod   = "X-AEUS-Request-Method"
	RequestProtocol = "X-AEUS-Request-Protocol"
	RequestClientIP = "X-AEUS-Request-Client-IP"
)

type (
	TeeReader interface {
		Get(string) string
	}

	TeeWriter interface {
		Set(string, string)
	}
)
