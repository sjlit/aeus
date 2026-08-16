package httpclient

import (
	"testing"
)

func TestWithParamsMerges(t *testing.T) {
	opts := newOptions()
	WithParams(map[string]string{"a": "1"})(opts)
	WithParams(map[string]string{"b": "2"})(opts)

	if opts.params["a"] != "1" {
		t.Errorf("params[a] = %q, want 1", opts.params["a"])
	}
	if opts.params["b"] != "2" {
		t.Errorf("params[b] = %q, want 2", opts.params["b"])
	}
}
