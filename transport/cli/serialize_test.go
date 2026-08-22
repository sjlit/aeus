package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sjlit/aeus/pkg/bytepool"
)

// TestSerializeMap_ReturnedBytesIndependentOfPool verifies that the
// slice returned by serializeMap remains valid after the internal
// bytepool buffer has been put back into the pool. If serializeMap
// returned buffer.Bytes() directly (without copying), the pool can
// hand the same buffer back to a subsequent caller and overwrite
// the returned slice — a use-after-free that corrupts output in
// production.
//
// The test snapshots the output, forces the pool to reuse its
// backing storage, then asserts the snapshot is unchanged.
func TestSerializeMap_ReturnedBytesIndependentOfPool(t *testing.T) {
	for iter := range 5 {
		m := map[any]any{"key": iter, "label": fmt.Sprintf("iter-%d", iter)}
		out, err := serializeMap(m)
		if err != nil {
			t.Fatalf("iter %d: serializeMap: %v", iter, err)
		}
		// Snapshot before pool reuse so we can detect mutation later.
		// The marker check guards against an empty/garbled snapshot
		// that would otherwise pass the corruption check trivially.
		want := string(out)
		if !strings.Contains(want, fmt.Sprintf("iter-%d", iter)) {
			t.Fatalf("iter %d: serializeMap output missing marker: %q", iter, out)
		}

		// One GetBuffer/PutBuffer round trip is enough to recycle the
		// backing array (the pool is LIFO); we do a few for safety.
		for j := range 3 {
			b := bytepool.GetBuffer()
			b.WriteString(strings.Repeat(string(rune('X'+j)), 4096))
			bytepool.PutBuffer(b)
		}

		if got := string(out); got != want {
			t.Errorf("iter %d: returned slice was corrupted by pool reuse:\n got: %q\nwant: %q",
				iter, got, want)
		}
	}
}

// TestPrintArray_ReturnedBytesIndependentOfPool is the printArray
// counterpart of the serializeMap test above — same use-after-free
// shape, same fix (pooledBytes clone), same regression coverage.
func TestPrintArray_ReturnedBytesIndependentOfPool(t *testing.T) {
	out := printArray([][]any{{"header1", "header2"}, {"v1", "v2"}})
	want := string(out)
	if want == "" {
		t.Fatalf("printArray returned empty output")
	}

	for j := range 3 {
		b := bytepool.GetBuffer()
		b.WriteString(strings.Repeat(string(rune('X'+j)), 4096))
		bytepool.PutBuffer(b)
	}

	if got := string(out); got != want {
		t.Errorf("returned slice was corrupted by pool reuse:\n got: %q\nwant: %q", got, want)
	}
}

// TestSerializeMap_TwoCallsIndependent verifies that two consecutive
// serializeMap calls do not share storage. With the bug, the second
// call's buffer.Bytes() aliases the same backing array as the first
// call, so writing into the pool buffer for the second call also
// mutates the slice returned by the first call.
func TestSerializeMap_TwoCallsIndependent(t *testing.T) {
	out1, err := serializeMap(map[any]any{"first": "alpha"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = serializeMap(map[any]any{"second": "beta"}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(out1), "alpha") {
		t.Errorf("out1 corrupted by subsequent serializeMap call:\n got: %q\nwant contains: alpha",
			out1)
	}
}
