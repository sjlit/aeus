package http

import (
	"context"
	"net/http"
	"net/url"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

var (
	ctxPool sync.Pool
)

type Context struct {
	ctx *gin.Context
}

func (c *Context) Context() context.Context {
	return c.ctx.Request.Context()
}

func (c *Context) Gin() *gin.Context {
	return c.ctx
}

func (c *Context) Request() *http.Request {
	return c.ctx.Request
}

func (c *Context) Response() http.ResponseWriter {
	return c.ctx.Writer
}

func (c *Context) ClientIP() string {
	return c.ctx.ClientIP()
}

func (c *Context) Param(key string) string {
	return c.ctx.Param(key)
}

func (c *Context) Query(key string) string {
	return c.ctx.Query(key)
}

func (c *Context) Bind(val any) (err error) {
	// if params exists, try bind params first
	if len(c.ctx.Params) > 0 {
		qs := url.Values{}
		for _, p := range c.ctx.Params {
			qs.Set(p.Key, p.Value)
		}
		if err = binding.MapFormWithTag(val, qs, "json"); err != nil {
			return err
		}
	}
	if c.Request().Method == http.MethodGet {
		values := c.Request().URL.Query()
		return binding.MapFormWithTag(val, values, "json")
	}
	return c.ctx.Bind(val)
}

func (c *Context) Error(code int, message string) (err error) {
	r := newResponse(code, message, nil)
	c.ctx.JSON(http.StatusOK, r)
	return
}

func (c *Context) Success(value any) (err error) {
	// P0 integration spec §1.2: a successful response carries the
	// business code OK (0) on the wire, not the HTTP status (200).
	// HTTP status stays 200 because that's the transport-level
	// signal — but the envelope's `code` is what the frontend
	// interceptor branches on.
	r := newResponse(0, "", value)
	c.ctx.JSON(http.StatusOK, r)
	return
}

func (c *Context) reset(ctx *gin.Context) {
	c.ctx = ctx
}

func newContext(ctx *gin.Context) *Context {
	v := ctxPool.Get()
	if v == nil {
		return &Context{
			ctx: ctx,
		}
	}
	if c, ok := v.(*Context); ok {
		c.reset(ctx)
		return c
	}
	return &Context{ctx: ctx}
}

func putContext(c *Context) {
	c.ctx = nil
	ctxPool.Put(c)
}
