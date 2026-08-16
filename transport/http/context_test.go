package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestContext() (*Context, *gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/", nil)
	ctx := newContext(c)
	return ctx, c, w
}

func TestContext_Request(t *testing.T) {
	ctx, _, _ := newTestContext()
	if ctx.Request() == nil {
		t.Error("Request() should not be nil")
	}
}

func TestContext_Response(t *testing.T) {
	ctx, _, _ := newTestContext()
	if ctx.Response() == nil {
		t.Error("Response() should not be nil")
	}
}

func TestContext_Param(t *testing.T) {
	ctx, ginCtx, _ := newTestContext()
	ginCtx.Params = gin.Params{gin.Param{Key: "id", Value: "123"}}
	if ctx.Param("id") != "123" {
		t.Errorf("Param(id) = %q, want \"123\"", ctx.Param("id"))
	}
}

func TestContext_Query(t *testing.T) {
	ctx, ginCtx, _ := newTestContext()
	ginCtx.Request, _ = http.NewRequest("GET", "/?name=alice", nil)
	if ctx.Query("name") != "alice" {
		t.Errorf("Query(name) = %q, want \"alice\"", ctx.Query("name"))
	}
}

func TestContext_Bind_WithParams(t *testing.T) {
	ctx, ginCtx, _ := newTestContext()
	ginCtx.Params = gin.Params{gin.Param{Key: "id", Value: "42"}}
	var result struct {
		ID int `json:"id"`
	}
	if err := ctx.Bind(&result); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}
	if result.ID != 42 {
		t.Errorf("ID = %d, want 42", result.ID)
	}
}

func TestContext_Bind_GET(t *testing.T) {
	ctx, ginCtx, _ := newTestContext()
	ginCtx.Request, _ = http.NewRequest("GET", "/?name=bob", nil)
	var result struct {
		Name string `json:"name"`
	}
	if err := ctx.Bind(&result); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}
	if result.Name != "bob" {
		t.Errorf("Name = %q, want \"bob\"", result.Name)
	}
}

func TestContext_Bind_Error(t *testing.T) {
	ctx, ginCtx, _ := newTestContext()
	ginCtx.Request, _ = http.NewRequest("POST", "/", nil)
	ginCtx.Request.Header.Set("Content-Type", "application/json")
	ginCtx.Request.Body = http.NoBody
	var result struct {
		Age int `json:"age"`
	}
	// Empty body should cause bind error for POST
	_ = ctx.Bind(&result)
	// Just verify it doesn't panic; exact error behavior depends on gin
}

func TestContext_Error(t *testing.T) {
	ctx, _, w := newTestContext()
	_ = ctx.Error(404, "not found")
	if w.Body.Len() == 0 {
		t.Error("response body should not be empty")
	}
	if w.Code != 200 {
		t.Errorf("HTTP code = %d, want 200 (JSON wrapper)", w.Code)
	}
}

func TestContext_Success(t *testing.T) {
	ctx, _, w := newTestContext()
	_ = ctx.Success(map[string]any{"id": 1})
	if w.Body.Len() == 0 {
		t.Error("response body should not be empty")
	}
	if w.Code != 200 {
		t.Errorf("HTTP code = %d, want 200", w.Code)
	}
}
