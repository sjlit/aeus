package registry

import "testing"

// TestWithRegisterContext verifies the correctly-spelled RegisterOption
// builder wires a context.Context into RegisterOptions.
func TestWithRegisterContext(t *testing.T) {
	ctx := t.Context()
	opts := NewRegisterOption(WithRegisterContext(ctx))
	if opts.Context != ctx {
		t.Fatal("WithRegisterContext did not propagate ctx to RegisterOptions")
	}
}
