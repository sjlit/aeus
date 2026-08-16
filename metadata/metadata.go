package metadata

import (
	"context"
	"iter"
	"maps"
	"strings"
)

type metadataKey struct{}

// Metadata is our way of representing request headers internally.
// They're used at the RPC level and translate back and forth
// from Transport headers.

type Metadata struct {
	teeReader TeeReader
	teeWriter TeeWriter
	variables map[string]string
}

func canonicalMetadataKey(key string) string {
	return strings.ToLower(key)
}

// TeeReader sets the tee reader.
func (m *Metadata) TeeReader(r TeeReader) {
	m.teeReader = r
}

// TeeWriter sets the tee writer.
func (m *Metadata) TeeWriter(w TeeWriter) {
	m.teeWriter = w
}

// Has returns true if the metadata contains the given key.
func (m *Metadata) Has(key string) bool {
	_, ok := m.Get(key)
	return ok
}

// Get returns the first value associated with the given key.
func (m *Metadata) Get(key string) (string, bool) {
	key = canonicalMetadataKey(key)
	val, ok := m.variables[key]
	if !ok && m.teeReader != nil {
		if val = m.teeReader.Get(key); val != "" {
			ok = true
		}
	}
	return val, ok
}

// Set sets a metadata key/value pair.
func (m *Metadata) Set(key, val string) {
	if m.variables == nil {
		m.variables = make(map[string]string)
	}
	key = canonicalMetadataKey(key)
	m.variables[key] = val
	if m.teeWriter != nil {
		m.teeWriter.Set(key, val)
	}
}

// Delete removes a key from the metadata.
func (m *Metadata) Delete(key string) {

	key = canonicalMetadataKey(key)
	if m.variables != nil {
		delete(m.variables, key)
	}
	if m.teeWriter != nil {
		m.teeWriter.Set(key, "")
	}
}

// Keys returns a sequence of the metadata keys.
func (m *Metadata) Keys() iter.Seq[string] {
	return func(yield func(string) bool) {
		for k := range m.variables {
			if !yield(k) {
				return
			}
		}
	}
}

// Delete key from metadata.
func Delete(ctx context.Context, k string) context.Context {
	return Set(ctx, k, "")
}

// Set add key with val to metadata.
func Set(ctx context.Context, k, v string) context.Context {
	md := FromContext(ctx)
	k = canonicalMetadataKey(k)
	if v == "" {
		md.Delete(k)
	} else {
		md.Set(k, v)
	}
	return context.WithValue(ctx, metadataKey{}, md)
}

// Get returns a single value from metadata in the context.
func Get(ctx context.Context, key string) (string, bool) {
	md := FromContext(ctx)
	key = canonicalMetadataKey(key)
	val, ok := md.Get(key)
	return val, ok
}

// FromContext returns metadata from the given context.
func FromContext(ctx context.Context) *Metadata {
	md, ok := ctx.Value(metadataKey{}).(*Metadata)
	if !ok {
		return New()
	}
	return md
}

// NewContext creates a new context with the given metadata.
func NewContext(ctx context.Context, md *Metadata) context.Context {
	return context.WithValue(ctx, metadataKey{}, md)
}

// MergeContext merges metadata to existing metadata, overwriting if specified.
func MergeContext(ctx context.Context, patchMd *Metadata, overwrite bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := New()
	// FromContext returns a *Metadata; copy its entries so the original
	// context is never mutated.
	maps.Copy(cmd.variables, FromContext(ctx).variables)
	if patchMd != nil {
		for k, v := range patchMd.variables {
			// an empty value means "delete", matching Set(ctx, k, "") semantics
			if v == "" {
				delete(cmd.variables, k)
				continue
			}
			if _, ok := cmd.variables[k]; ok && !overwrite {
				// skip
			} else {
				cmd.variables[k] = v
			}
		}
	}
	return context.WithValue(ctx, metadataKey{}, cmd)
}

func New() *Metadata {
	return &Metadata{
		variables: make(map[string]string, 16),
	}
}
