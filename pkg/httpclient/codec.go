package httpclient

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

func encodeBody(data any) (r io.Reader, contentType string, err error) {
	var (
		buf []byte
	)
	switch v := data.(type) {
	case string:
		r = strings.NewReader(v)
		contentType = "application/x-www-form-urlencoded"
	case []byte:
		r = bytes.NewReader(v)
		contentType = "application/x-www-form-urlencoded"
	default:
		if buf, err = json.Marshal(v); err == nil {
			r = bytes.NewReader(buf)
			contentType = "application/json"
		}
	}
	return
}
