package httpclient

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

func decodeResponse(res *http.Response, result any) error {
	contentType := strings.ToLower(res.Header.Get("Content-Type"))
	extName := path.Ext(res.Request.URL.String())
	if strings.Contains(contentType, JSON) || extName == ".json" {
		return json.NewDecoder(res.Body).Decode(result)
	}
	if strings.Contains(contentType, XML) || extName == ".xml" {
		return xml.NewDecoder(res.Body).Decode(result)
	}
	return fmt.Errorf("unsupported content type: %s", contentType)
}

func doHttpRequest(req *http.Request, opts *options) (res *http.Response, err error) {
	if opts.browserHeaders {
		if req.Header.Get("User-Agent") == "" {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/111.0.0.0 Safari/537.36 Edg/111.0.1661.54")
		}
		if req.Header.Get("Referer") == "" {
			req.Header.Set("Referer", req.URL.String())
		}
	}
	return opts.client.Do(req)
}

// Get performs a GET request to the specified URL with optional parameters and headers.
//
// Deprecated: Use Client.Get() for better control and consistency. Global functions
// have inconsistent behavior and will be removed in a future version.
func Get(ctx context.Context, urlString string, cbs ...Option) (res *http.Response, err error) {
	var (
		uri *url.URL
		req *http.Request
	)
	opts := newOptions()
	for _, cb := range cbs {
		cb(opts)
	}
	if uri, err = url.Parse(urlString); err != nil {
		return
	}
	if opts.params != nil {
		qs := uri.Query()
		for k, v := range opts.params {
			qs.Set(k, v)
		}
		uri.RawQuery = qs.Encode()
	}
	if req, err = http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil); err != nil {
		return
	}
	if opts.header != nil {
		for k, v := range opts.header {
			req.Header.Set(k, v)
		}
	}
	return doHttpRequest(req, opts)
}

// Post performs a POST request to the specified URL with optional parameters, headers, and data.
//
// Deprecated: Use Client.Post() for better control and consistency. Global functions
// have inconsistent behavior and will be removed in a future version.
func Post(ctx context.Context, urlString string, cbs ...Option) (res *http.Response, err error) {
	var (
		uri         *url.URL
		req         *http.Request
		contentType string
		reader      io.Reader
	)
	opts := newOptions()
	for _, cb := range cbs {
		cb(opts)
	}
	if uri, err = url.Parse(urlString); err != nil {
		return
	}
	if opts.params != nil {
		qs := uri.Query()
		for k, v := range opts.params {
			qs.Set(k, v)
		}
		uri.RawQuery = qs.Encode()
	}
	if opts.body != nil {
		if reader, contentType, err = encodeBody(opts.body); err != nil {
			return
		}
	}
	if req, err = http.NewRequestWithContext(ctx, http.MethodPost, uri.String(), reader); err != nil {
		return
	}
	if opts.header != nil {
		for k, v := range opts.header {
			req.Header.Set(k, v)
		}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return doHttpRequest(req, opts)
}

// Do performs a request to the specified URL with optional parameters, headers, and data.
//
// Deprecated: Use Client.Get/Post/Do chain for better control and consistency. Global
// functions have inconsistent behavior and will be removed in a future version.
func Do(ctx context.Context, urlString string, result any, cbs ...Option) (err error) {
	var (
		contentType string
		reader      io.Reader
		uri         *url.URL
		res         *http.Response
		req         *http.Request
	)
	opts := newOptions()
	for _, cb := range cbs {
		cb(opts)
	}
	if uri, err = url.Parse(urlString); err != nil {
		return
	}
	if opts.params != nil {
		qs := uri.Query()
		for k, v := range opts.params {
			qs.Set(k, v)
		}
		uri.RawQuery = qs.Encode()
	}
	if opts.body != nil {
		if reader, contentType, err = encodeBody(opts.body); err != nil {
			return
		}
	}
	if req, err = http.NewRequestWithContext(ctx, opts.method, uri.String(), reader); err != nil {
		return
	}
	if opts.header != nil {
		for k, v := range opts.header {
			req.Header.Set(k, v)
		}
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if res, err = doHttpRequest(req, opts); err != nil {
		return
	}
	defer func() {
		_ = res.Body.Close()
	}()
	if res.StatusCode/100 != 2 {
		err = fmt.Errorf("unexpected status %s(%d)", res.Status, res.StatusCode)
		return
	}
	//don't care response
	if result == nil {
		return nil
	}
	err = decodeResponse(res, result)
	return
}
