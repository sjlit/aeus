package cli

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/url"
	"sync"

	"github.com/go-playground/form/v4"
)

type Context struct {
	ID        int64
	seq       uint16
	ctx       context.Context
	wc        io.WriteCloser
	params    map[string]string
	locker    sync.RWMutex
	variables map[string]any
	args      []string
}

func (ctx *Context) reset(id int64, wc io.WriteCloser) {
	ctx.ID = id
	ctx.wc = wc
	ctx.seq = 0
	ctx.ctx = context.Background()
	ctx.args = make([]string, 0)
	ctx.params = make(map[string]string)
	ctx.variables = make(map[string]any)
}

func (ctx *Context) setArgs(args []string) {
	ctx.args = args
}

func (ctx *Context) setParam(ps map[string]string) {
	ctx.params = ps
}

var formDecoder = form.NewDecoder()

func init() {
	formDecoder.SetTagName("json")
}

func (ctx *Context) Bind(val any) (err error) {
	qs := url.Values{}
	for k, v := range ctx.params {
		qs.Set(k, v)
	}
	return formDecoder.Decode(val, qs)
}

func (ctx *Context) setContext(c context.Context) {
	ctx.ctx = c
}

func (ctx *Context) Context() context.Context {
	return ctx.ctx
}

func (ctx *Context) Argument(index int) string {
	if index >= len(ctx.args) || index < 0 {
		return ""
	}
	return ctx.args[index]
}

func (ctx *Context) Param(s string) string {
	if v, ok := ctx.params[s]; ok {
		return v
	}
	return ""
}

func (ctx *Context) SetValue(name string, value any) {
	ctx.locker.Lock()
	if ctx.variables == nil {
		ctx.variables = make(map[string]any)
	}
	ctx.variables[name] = value
	ctx.locker.Unlock()
}

func (ctx *Context) Value(name string) (val any, ok bool) {
	ctx.locker.RLock()
	defer ctx.locker.RUnlock()
	val, ok = ctx.variables[name]
	return
}

func (ctx *Context) Success(v any) (err error) {
	return ctx.send(responsePayload{Type: PacketTypeCommand, Data: v})
}

func (ctx *Context) Error(code int, message string) (err error) {
	return ctx.send(responsePayload{Type: PacketTypeCommand, Code: code, Message: message})
}

func (ctx *Context) Close() (err error) {
	return ctx.wc.Close()
}

func (ctx *Context) send(res responsePayload) (err error) {
	if res.Code > 0 {
		return writeFrame(ctx.wc, &Frame{
			Feature: Feature,
			Type:    res.Type,
			Seq:     ctx.seq,
			Flag:    FlagComplete,
			Error:   fmt.Sprintf("ERROR(%d): %s", res.Code, res.Message),
		})
	}
	buf, err := ctx.encode(res.Data)
	if err != nil {
		return
	}
	return ctx.writeChunked(res.Type, buf)
}

// encode produces the wire bytes for res.Data, falling back through the
// registered encoder or the generic serializer.
func (ctx *Context) encode(data any) ([]byte, error) {
	if data == nil {
		return OK, nil
	}
	if m, ok := data.(encoder); ok {
		return m.Marshal()
	}
	return serialize(data)
}

// writeChunked writes buf to the underlying writer as one or more frames,
// using FlagPortion for all but the final frame which is marked FlagComplete.
func (ctx *Context) writeChunked(t uint8, buf []byte) error {
	chunkSize := math.MaxInt16 - 1
	offset := 0
	for i := 0; i < len(buf)/chunkSize; i++ {
		if err := writeFrame(ctx.wc, newFrame(t, FlagPortion, ctx.seq, 0, buf[offset:chunkSize+offset])); err != nil {
			return err
		}
		offset += chunkSize
	}
	return writeFrame(ctx.wc, newFrame(t, FlagComplete, ctx.seq, 0, buf[offset:]))
}
