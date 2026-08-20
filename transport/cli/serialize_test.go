package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/sjlit/aeus/pkg/bytepool"
)

// TestSerializeMap_ReturnedBytesIndependentOfPool verifies that the
// slice returned by serializeMap remains valid after the internal
// bytepool buffer has been put back into the pool. If serializeMap
// returns buffer.Bytes() directly (without copying), the pool can
// hand the same buffer back to a subsequent caller and overwrite
// the returned slice — a use-after-free that corrupts output in
// production.
//
// The test snapshots the output, forces the pool to reuse its
// backing storage, then asserts the snapshot is unchanged.
func TestSerializeMap_ReturnedBytesIndependentOfPool(t *testing.T) {
	for iter := range 50 {
		m := map[any]any{"key": iter, "label": fmt.Sprintf("iter-%d", iter)}
		out, err := serializeMap(m)
		if err != nil {
			t.Fatalf("iter %d: serializeMap: %v", iter, err)
		}
		// Snapshot before pool reuse so we can detect mutation later.
		want := string(out)
		if !strings.Contains(want, fmt.Sprintf("iter-%d", iter)) {
			t.Fatalf("iter %d: serializeMap output missing marker: %q", iter, out)
		}

		// Force the pool to reuse its backing arrays by acquiring and
		// filling buffers repeatedly. The original buffer's storage
		// will be overwritten somewhere in this loop.
		for j := range 50 {
			b := bytepool.GetBuffer()
			b.Write(bytes.Repeat([]byte{'X' + byte(j%16)}, 4096))
			bytepool.PutBuffer(b)
		}

		// If serializeMap returned a slice aliased to the pool buffer,
		// the content has been corrupted.
		if got := string(out); got != want {
			t.Errorf("iter %d: returned slice was corrupted by pool reuse:\n got: %q\nwant: %q",
				iter, got, want)
		}
	}
}

// TestSerializeMap_TwoCallsIndependent verifies that two consecutive
// serializeMap calls do not share storage. With the bug, the second
// call's buffer.Bytes() aliases the same backing array as the first
// call, so writing into the pool buffer for the second call also
// mutates the slice returned by the first call.
func TestSerializeMap_TwoCallsIndependent(t *testing.T) {
	m1 := map[any]any{"first": "alpha"}
	out1, err := serializeMap(m1)
	if err != nil {
		t.Fatal(err)
	}

	m2 := map[any]any{"second": "beta"}
	_, err = serializeMap(m2)
	if err != nil {
		t.Fatal(err)
	}

	// out1 must still describe m1, not be overwritten by m2's write.
	if !strings.Contains(string(out1), "alpha") {
		t.Errorf("out1 corrupted by subsequent serializeMap call:\n got: %q\nwant contains: alpha",
			out1)
	}
}
