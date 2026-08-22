package http

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/sjlit/aeus/pkg/errs"
)

func startFixTest(t *testing.T, svr *Server) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = svr.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)
	ep, err := svr.Endpoint(ctx)
	if err != nil {
		t.Fatalf("endpoint: %v", err)
	}
	return ep
}

// 回归 #6: prefix 沙箱外的文件不可达
func TestFS_PrefixBoundaryEscapeBlocked(t *testing.T) {
	mfs := fstest.MapFS{
		"staticfoo.txt":    &fstest.MapFile{Data: []byte("OUTSIDE")},
		"static/inner.txt": &fstest.MapFile{Data: []byte("INSIDE")},
	}
	fsys := newFS(time.Now(), FileSystem(mfs))
	fsys.SetPrefix("static")

	if f, err := fsys.Open("/staticfoo.txt"); err == nil {
		f.Close()
		t.Fatal("file outside prefix was reachable")
	}
	if f, err := fsys.Open("/static/../staticfoo.txt"); err == nil {
		f.Close()
		t.Fatal("traversal outside prefix was reachable")
	}
	f, err := fsys.Open("/inner.txt")
	if err != nil {
		t.Fatalf("prefixed file unreachable: %v", err)
	}
	b, _ := io.ReadAll(f)
	f.Close()
	if string(b) != "INSIDE" {
		t.Fatalf("got %q, want INSIDE", string(b))
	}
}

// 回归 #7: wrapped errs.Error 保留业务 code 与 HTTP 状态
func TestServer_WrappedErrsErrorKeepsCode(t *testing.T) {
	svr := New(WithAddress(":0"), WithDebug(true))
	svr.GET("/wrapped", func(ctx *Context) error {
		return fmt.Errorf("usecase failed: %w", errs.New(errs.CodeNotFound, "user not found"))
	})
	ep := startFixTest(t, svr)

	resp, err := http.Get(ep + "/wrapped")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", resp.StatusCode)
	}
	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	if int(r.Code) != int(errs.CodeNotFound) {
		t.Fatalf("got code %d, want %d", r.Code, errs.CodeNotFound)
	}
}

// 回归 #8: Success 后返回错误只产出一个合法 JSON 信封
func TestServer_NoDoubleEnvelopeAfterWritten(t *testing.T) {
	svr := New(WithAddress(":0"), WithDebug(true))
	svr.GET("/dual", func(ctx *Context) error {
		if err := ctx.Success("partial data"); err != nil {
			return err
		}
		return errs.New(errs.CodeInternal, "late failure")
	})
	ep := startFixTest(t, svr)

	resp, err := http.Get(ep + "/dual")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("response is not a single JSON document: %v; body=%s", err, body)
	}
	if r.Data != "partial data" {
		t.Fatalf("unexpected data: %v", r.Data)
	}
}

// 回归 #1: .gz 回退仅在客户端接受 gzip 时启用，并带 Content-Encoding
func TestServer_GzFallbackContentEncoding(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte("<html>hello</html>"))
	zw.Close()

	mfs := fstest.MapFS{"web/index.html.gz": &fstest.MapFile{Data: buf.Bytes()}}
	svr := New(WithAddress(":0"), WithDebug(true))
	svr.Webroot("web", false, FileSystem(mfs))
	ep := startFixTest(t, svr)

	// 不接受 gzip: 应回退 404，而不是下发裸 gzip 字节
	// （显式设置 identity，避免 net/http 自动附加 Accept-Encoding: gzip）
	req0, _ := http.NewRequest("GET", ep+"/index.html", nil)
	req0.Header.Set("Accept-Encoding", "identity")
	resp, err := http.DefaultTransport.RoundTrip(req0)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("no-gzip client: got status %d, want 404", resp.StatusCode)
	}
	if bytes.HasPrefix(body, []byte{0x1f, 0x8b}) {
		t.Fatal("raw gzip bytes served without Accept-Encoding")
	}

	// 接受 gzip: 应返回解压后内容且带 Content-Encoding
	req, _ := http.NewRequest("GET", ep+"/index.html", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp2, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	raw, _ := io.ReadAll(resp2.Body)
	if ce := resp2.Header.Get("Content-Encoding"); ce != "gzip" {
		t.Fatalf("got Content-Encoding %q, want gzip", ce)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("body is not valid gzip: %v", err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != "<html>hello</html>" {
		t.Fatalf("decoded body %q", string(got))
	}
}

// 回归 #2: Range + gzip 不再产出与 Content-Range 不符的损坏响应
func TestServer_RangeDroppedUnderGzip(t *testing.T) {
	css := strings.Repeat("body{color:red}", 1000) // > 8192
	mfs := fstest.MapFS{"web/app.css": &fstest.MapFile{Data: []byte(css)}}
	svr := New(WithAddress(":0"), WithDebug(true))
	svr.Webroot("web", true, FileSystem(mfs))
	ep := startFixTest(t, svr)

	req, _ := http.NewRequest("GET", ep+"/app.css", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Range", "bytes=0-14")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got status %d, want 200 (full body)", resp.StatusCode)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		t.Fatalf("unexpected Content-Range %q under gzip", cr)
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("body is not valid gzip: %v", err)
	}
	got, _ := io.ReadAll(zr)
	if len(got) != len(css) {
		t.Fatalf("decoded %d bytes, want full %d", len(got), len(css))
	}
}

// 回归 #5: 通配符 Origin 下不再下发 Allow-Credentials
func TestCORS_NoWildcardCredentials(t *testing.T) {
	svr := New(WithAddress(":0"), WithDebug(true), WithCORS())
	ep := startFixTest(t, svr)

	req, _ := http.NewRequest("OPTIONS", ep+"/any", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if acac := resp.Header.Get("Access-Control-Allow-Credentials"); acac != "" {
		t.Fatalf("Allow-Credentials %q sent alongside wildcard origin", acac)
	}
	if acao := resp.Header.Get("Access-Control-Allow-Origin"); acao != "*" {
		t.Fatalf("got ACAO %q, want *", acao)
	}
}
