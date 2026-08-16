package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

const (
	JSON = "application/json"
	XML  = "application/xml"

	plainTextType   = "text/plain; charset=utf-8"
	jsonContentType = "application/json"
	formContentType = "application/x-www-form-urlencoded"
)

const maxErrorBodySize = 64 * 1024 // 64KB

var (
	jsonCheck = regexp.MustCompile(`(?i:(application|text)/(json|.*\+json|json\-.*)(;|$))`)
	xmlCheck  = regexp.MustCompile(`(?i:(application|text)/(xml|.*\+xml)(;|$))`)
)

type Request struct {
	context       context.Context
	method        string
	uri           string
	url           *url.URL
	body          any
	query         url.Values
	formData      url.Values
	header        http.Header
	contentType   string
	authorization Authorization
	client        *Client
	rawRequest    *http.Request
	rawResponse   *http.Response
}

func (r *Request) detectContentType(body any) string {
	if body == nil {
		return ""
	}
	contentType := plainTextType
	value := reflect.Indirect(reflect.ValueOf(body))
	if !value.IsValid() {
		return ""
	}
	kind := value.Type().Kind()
	switch kind {
	case reflect.Struct, reflect.Map:
		contentType = jsonContentType
	case reflect.String:
		contentType = plainTextType
	default:
		if b, ok := body.([]byte); ok {
			contentType = http.DetectContentType(b)
		} else if kind == reflect.Slice {
			contentType = jsonContentType
		}
	}
	return contentType
}

func (r *Request) readRequestBody(contentType string, body any) (reader io.Reader, err error) {
	if reader, ok := r.body.(io.Reader); ok {
		return reader, nil
	}
	if buf, ok := r.body.([]byte); ok {
		if len(buf) > 0 {
			return bytes.NewReader(buf), nil
		}
		return nil, nil
	}
	if s, ok := r.body.(string); ok {
		buf := []byte(s)
		if len(buf) > 0 {
			return bytes.NewReader(buf), nil
		}
		return nil, nil
	}

	kind := reflect.Indirect(reflect.ValueOf(body)).Type().Kind()
	if jsonCheck.MatchString(contentType) && (kind == reflect.Struct || kind == reflect.Map || kind == reflect.Slice) {
		buf, err := json.Marshal(r.body)
		if err != nil {
			return nil, err
		}
		if len(buf) > 0 {
			return bytes.NewReader(buf), nil
		}
		return nil, nil
	}
	if xmlCheck.MatchString(contentType) && (kind == reflect.Struct) {
		buf, err := xml.Marshal(r.body)
		if err != nil {
			return nil, err
		}
		if len(buf) > 0 {
			return bytes.NewReader(buf), nil
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unsupported content type %s for body type %T", contentType, r.body)
}

func (r *Request) SetContext(ctx context.Context) *Request {
	r.context = ctx
	return r
}

func (r *Request) AddQuery(k, v string) *Request {
	r.query.Add(k, v)
	return r
}

func (r *Request) SetQuery(vs map[string]string) *Request {
	for k, v := range vs {
		r.query.Set(k, v)
	}
	return r
}

func (r *Request) AddFormData(k, v string) *Request {
	r.contentType = formContentType
	r.formData.Add(k, v)
	return r
}

func (r *Request) SetFormData(vs map[string]string) *Request {
	r.contentType = formContentType
	for k, v := range vs {
		r.formData.Set(k, v)
	}
	return r
}

func (r *Request) SetBody(v any) *Request {
	r.body = v
	return r
}

func (r *Request) SetContentType(v string) *Request {
	r.contentType = v
	return r
}

func (r *Request) AddHeader(k, v string) *Request {
	r.header.Add(k, v)
	return r
}

func (r *Request) SetHeader(h http.Header) *Request {
	r.header = h
	return r
}

func (r *Request) Do() (res *http.Response, err error) {
	var s string
	body := r.body
	s = r.formData.Encode()
	if len(s) > 0 && body == nil {
		body = s
	}
	r.url.RawQuery = r.query.Encode()
	uri := r.url.String()

	// Create a temporary request with resolved values
	tmp := *r
	tmp.body = body
	tmp.uri = uri
	return r.client.execute(&tmp)
}

func (r *Request) Response(v any) (err error) {
	var (
		res *http.Response
		buf []byte
	)
	if res, err = r.Do(); err != nil {
		return
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode/100 != 2 {
		limited := io.LimitReader(res.Body, maxErrorBodySize)
		if buf, err = io.ReadAll(limited); err == nil && len(buf) > 0 {
			err = fmt.Errorf("http response %s(%d): %s", res.Status, res.StatusCode, string(buf))
		} else {
			err = fmt.Errorf("http response %d: %s", res.StatusCode, res.Status)
		}
		return
	}
	err = decodeResponse(res, v)
	return
}

func (r *Request) Download(s string) (err error) {
	clean := filepath.Clean(s)
	if strings.Contains(clean, "..") {
		return fmt.Errorf("invalid download path: path traversal detected")
	}

	var (
		fp  *os.File
		res *http.Response
	)
	if res, err = r.Do(); err != nil {
		return
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if fp, err = os.OpenFile(clean, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644); err != nil {
		return
	}
	defer func() {
		_ = fp.Close()
	}()
	_, err = io.Copy(fp, res.Body)
	return
}

func newRequest(method string, uri string, client *Client) *Request {
	var (
		err error
	)
	r := &Request{
		context:  context.Background(),
		method:   method,
		uri:      uri,
		header:   make(http.Header),
		formData: make(url.Values),
		client:   client,
	}
	if r.url, err = url.Parse(uri); err == nil {
		r.query = r.url.Query()
	} else {
		r.query = make(url.Values)
	}
	return r
}
