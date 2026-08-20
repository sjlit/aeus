package cli

import (
	"bytes"
	"io"
	"strconv"
	"testing"
)

// readAllFrames decodes the wire output produced by writeChunked back
// into a slice of *Frame. The chunked-write helper writes frames with
// a fixed binary header; the inverse is exercised by readFrame.
func readAllFrames(t *testing.T, r io.Reader) []*Frame {
	t.Helper()
	var frames []*Frame
	for {
		f, err := readFrame(r)
		if err == io.EOF {
			return frames
		}
		if err != nil {
			t.Fatalf("readFrame: %v", err)
		}
		frames = append(frames, f)
	}
}

// writeCloserAdapter wraps an io.Writer so it satisfies io.WriteCloser.
type writeCloserAdapter struct{ io.Writer }

func (writeCloserAdapter) Close() error { return nil }

// TestWriteChunked covers two invariants for writeChunked:
//
//  1. No trailing empty FlagComplete frame when len(buf) is an exact
//     multiple of wireChunkSize. The original implementation's loop
//     ran k times (sending k FlagPortion frames) and then always
//     emitted one more FlagComplete frame, even when the final slice
//     was empty — producing k+1 frames and a zero-byte terminator that
//     the receiver could not distinguish from a legitimate empty payload.
//  2. The bytes reassembled from the chunked frames equal the original
//     input (no data loss or reordering across the chunk boundary).
//
// The {wireChunkSize, 2×wireChunkSize, 10×wireChunkSize} cases below
// are the regression-relevant boundaries — the buggy loop emitted an
// extra empty frame for each of them.
func TestWriteChunked(t *testing.T) {
	cases := []struct {
		size int
	}{
		{0},
		{1},
		{wireChunkSize - 1},
		{wireChunkSize},      // exact multiple
		{wireChunkSize + 1},
		{wireChunkSize * 2},  // exact multiple
		{wireChunkSize*2 + 1},
		{100},
		{wireChunkSize*2 + 17},
		{wireChunkSize * 5},
		{wireChunkSize * 10}, // exact multiple
	}

	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.size), func(t *testing.T) {
			var buf bytes.Buffer
			ctx := &Context{wc: writeCloserAdapter{Writer: &buf}}
			data := bytes.Repeat([]byte{'A'}, tc.size)
			if err := ctx.writeChunked(PacketTypeCommand, data); err != nil {
				t.Fatalf("writeChunked: %v", err)
			}

			frames := readAllFrames(t, &buf)
			wantFrames := 0
			if tc.size > 0 {
				wantFrames = (tc.size + wireChunkSize - 1) / wireChunkSize
			}
			if len(frames) != wantFrames {
				t.Errorf("len(frames) = %d, want %d", len(frames), wantFrames)
			}

			if tc.size > 0 && len(frames) > 0 {
				last := frames[len(frames)-1]
				if len(last.Data) == 0 {
					t.Errorf("last frame has empty Data (extra empty frame bug): flag=%d", last.Flag)
				}
				if last.Flag != FlagComplete {
					t.Errorf("last frame Flag = %d, want FlagComplete (%d)", last.Flag, FlagComplete)
				}
			}

			var got []byte
			for _, f := range frames {
				got = append(got, f.Data...)
			}
			if !bytes.Equal(got, data) {
				t.Errorf("reassembled data mismatch: got len=%d, want len=%d", len(got), len(data))
			}
		})
	}
}