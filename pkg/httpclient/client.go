package httpclient

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"

	"github.com/sjlit/aeus/infra/telemetry"
)

type (
	BeforeRequest func(client *http.Client, req *http.Request) (err error)
	AfterRequest  func(client *http.Client, req *http.Request, res *http.Response) (err error)

	Client struct {
		mu                  sync.RWMutex
		baseURL             string
		Authorization       Authorization
		client              *http.Client
		cookieJar           *cookiejar.Jar
		interceptorRequest  []BeforeRequest
		interceptorResponse []AfterRequest
		tracer              telemetry.Tracer
	}
)

var (
	DefaultClient = &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          64,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
		Timeout: time.Second * 30,
	}
)

func (client *Client) stashURI(urlPath string) string {
	var (
		pos int
	)
	client.mu.RLock()
	baseURL := client.baseURL
	client.mu.RUnlock()
	if len(urlPath) == 0 {
		return baseURL
	}
	if pos = strings.Index(urlPath, "//"); pos == -1 {
		if baseURL != "" {
			if urlPath[0] != '/' {
				urlPath = "/" + urlPath
			}
			return baseURL + urlPath
		}
	}
	return urlPath
}

func (client *Client) BeforeRequest(cb BeforeRequest) *Client {
	client.mu.Lock()
	client.interceptorRequest = append(client.interceptorRequest, cb)
	client.mu.Unlock()
	return client
}

func (client *Client) AfterRequest(cb AfterRequest) *Client {
	client.mu.Lock()
	client.interceptorResponse = append(client.interceptorResponse, cb)
	client.mu.Unlock()
	return client
}

func (client *Client) SetBaseURL(s string) *Client {
	client.mu.Lock()
	client.baseURL = strings.TrimSuffix(s, "/")
	client.mu.Unlock()
	return client
}

func (client *Client) SetCookieJar(cookieJar *cookiejar.Jar) *Client {
	client.mu.Lock()
	client.cookieJar = cookieJar
	client.client.Jar = cookieJar
	client.mu.Unlock()
	return client
}

func (client *Client) SetClient(httpClient *http.Client) *Client {
	client.mu.Lock()
	client.client = httpClient
	if client.cookieJar != nil {
		client.client.Jar = client.cookieJar
	}
	client.mu.Unlock()
	return client
}

func (client *Client) SetTransport(transport http.RoundTripper) *Client {
	client.mu.Lock()
	client.client.Transport = transport
	client.mu.Unlock()
	return client
}

func (client *Client) SetTracer(t telemetry.Tracer) *Client {
	client.mu.Lock()
	client.tracer = t
	client.mu.Unlock()
	return client
}

func (client *Client) Get(urlPath string) *Request {
	return newRequest(http.MethodGet, client.stashURI(urlPath), client)
}

func (client *Client) Put(urlPath string) *Request {
	return newRequest(http.MethodPut, client.stashURI(urlPath), client)
}

func (client *Client) Post(urlPath string) *Request {
	return newRequest(http.MethodPost, client.stashURI(urlPath), client)
}

func (client *Client) Delete(urlPath string) *Request {
	return newRequest(http.MethodDelete, client.stashURI(urlPath), client)
}

func (client *Client) execute(r *Request) (res *http.Response, err error) {
	var (
		reader io.Reader
	)
	if r.contentType == "" && r.body != nil {
		r.contentType = r.detectContentType(r.body)
	}
	if r.body != nil {
		if reader, err = r.readRequestBody(r.contentType, r.body); err != nil {
			return
		}
	}
	if r.rawRequest, err = http.NewRequest(r.method, r.uri, reader); err != nil {
		return
	}
	for k, vs := range r.header {
		for _, v := range vs {
			r.rawRequest.Header.Add(k, v)
		}
	}
	if r.contentType != "" {
		r.rawRequest.Header.Set("Content-Type", r.contentType)
	}

	client.mu.RLock()
	auth := client.Authorization
	reqInterceptors := make([]BeforeRequest, len(client.interceptorRequest))
	copy(reqInterceptors, client.interceptorRequest)
	resInterceptors := make([]AfterRequest, len(client.interceptorResponse))
	copy(resInterceptors, client.interceptorResponse)
	httpClient := client.client
	client.mu.RUnlock()

	if auth != nil {
		r.rawRequest.Header.Set("Authorization", auth.Token())
	}
	// Inject trace context into outgoing request headers
	if client.tracer != nil && r.context != nil {
		client.tracer.Inject(r.context, telemetry.HTTPHeaderCarrier{Header: r.rawRequest.Header})
	}
	if r.context != nil {
		r.rawRequest = r.rawRequest.WithContext(r.context)
	}
	for _, cb := range reqInterceptors {
		if err = cb(httpClient, r.rawRequest); err != nil {
			return
		}
	}
	if r.rawResponse, err = httpClient.Do(r.rawRequest); err != nil {
		return nil, err
	}
	for _, cb := range resInterceptors {
		if err = cb(httpClient, r.rawRequest, r.rawResponse); err != nil {
			_ = r.rawResponse.Body.Close()
			return
		}
	}
	return r.rawResponse, err
}

// New creates a new Client with an independent *http.Client built
// from a deep copy of DefaultClient's Transport. Each returned
// Client owns its own *http.Client, cookieJar, Transport and
// Timeout — SetTransport / SetClient / SetCookieJar on one client
// do not affect any other client or the package-level DefaultClient.
func New() *Client {
	httpClient := cloneDefaultClient()
	jar, _ := cookiejar.New(nil)
	httpClient.Jar = jar
	return &Client{
		client:              httpClient,
		cookieJar:           jar,
		interceptorRequest:  make([]BeforeRequest, 0, 10),
		interceptorResponse: make([]AfterRequest, 0, 10),
	}
}

// cloneDefaultClient returns a fresh *http.Client whose Transport is
// a copy of DefaultClient.Transport so per-client tweaks (timeout,
// dialer, TLS) do not leak into the package-level singleton. When
// DefaultClient.Transport is *http.Transport (the common case) the
// transport is deep-copied; otherwise the same transport pointer is
// reused because callers using custom RoundTrippers are expected to
// own and configure them.
func cloneDefaultClient() *http.Client {
	c := &http.Client{Timeout: DefaultClient.Timeout}
	if t, ok := DefaultClient.Transport.(*http.Transport); ok {
		c.Transport = t.Clone()
	} else {
		c.Transport = DefaultClient.Transport
	}
	return c
}
