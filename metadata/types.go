package metadata

const (
	RequestID       = "X-AEUS-Request-ID"
	RequestPath     = "X-AEUS-Request-Path"
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
