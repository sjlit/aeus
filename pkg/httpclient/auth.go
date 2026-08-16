package httpclient

import (
	"encoding/base64"
	"fmt"
)

type Authorization interface {
	Token() string
}

// BasicAuth implements HTTP Basic Authentication.
// Note: Username and Password should not contain the ':' character,
// as the token is formed by base64(Username + ":" + Password) without escaping.
type BasicAuth struct {
	Username string
	Password string
}

type BearerAuth struct {
	AccessToken string
}

func (auth *BasicAuth) Token() string {
	return fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(auth.Username+":"+auth.Password)))
}

func (auth *BearerAuth) Token() string {
	return fmt.Sprintf("Bearer %s", auth.AccessToken)
}
