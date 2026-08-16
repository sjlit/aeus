package telemetry

import (
	"net/http"

	"github.com/sjlit/aeus/metadata"
)

// MetadataCarrier adapts aeus metadata to TextMapReader/Writer.
type MetadataCarrier struct {
	MD *metadata.Metadata
}

func (c MetadataCarrier) Get(key string) string {
	v, _ := c.MD.Get(key)
	return v
}

func (c MetadataCarrier) Set(key, value string) {
	c.MD.Set(key, value)
}

// HTTPHeaderCarrier adapts http.Header to TextMapReader/Writer.
type HTTPHeaderCarrier struct {
	Header http.Header
}

func (c HTTPHeaderCarrier) Get(key string) string {
	return c.Header.Get(key)
}

func (c HTTPHeaderCarrier) Set(key, value string) {
	c.Header.Set(key, value)
}
