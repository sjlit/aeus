package cli

import (
	"bytes"
	"io"
	"testing"
)

// chunkSize mirrors Context.writeChunked's internal chunk size
// (math.MaxInt16 - 1 = 32766). Kept here so a future change to the
// production constant surfaces in the tests immediately.
const chunkSize = 32766

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

// TestWriteChunked_NoExtraEmptyFrame verifies that writeChunked does
// not emit a trailing empty FlagComplete frame when len(buf) is an
// exact multiple of the chunk size. The original implementation's
// loop ran k times (sending k FlagPortion frames) and then always
// emitted one more FlagComplete frame, even when the final slice was
// empty — producing k+1 frames and a zero-byte terminator that the
// receiver could not distinguish from a legitimate empty payload.
func TestWriteChunked_NoExtraEmptyFrame(t *testing.T) {
	cases := []struct {
		name       string
		size       int
		wantFrames int
	}{
		{"empty", 0, 0},
		{"single byte", 1, 1},
		{"just under one chunk", chunkSize - 1, 1},
		{"exactly one chunk", chunkSize, 1}, // exact multiple → was the buggy case
		{"one over one chunk", chunkSize + 1, 2},
		{"exactly two chunks", chunkSize * 2, 2}, // exact multiple → was the buggy case
		{"two chunks plus one", chunkSize*2 + 1, 3},
		{"large multiple", chunkSize * 10, 10}, // exact multiple → was the buggy case
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			ctx := &Context{wc: writeCloserAdapter{Writer: &buf}}
			data := bytes.Repeat([]byte{'A'}, tc.size)
			if err := ctx.writeChunked(PacketTypeCommand, data); err != nil {
				t.Fatalf("writeChunked: %v", err)
			}

			frames := readAllFrames(t, &buf)
			if len(frames) != tc.wantFrames {
				t.Errorf("len(frames) = %d, want %d", len(frames), tc.wantFrames)
			}

			// For size > 0, the LAST frame must carry the trailing data —
			// never be empty.
			if tc.size > 0 && len(frames) > 0 {
				last := frames[len(frames)-1]
				if len(last.Data) == 0 {
					t.Errorf("last frame has empty Data (extra empty frame bug): flag=%d", last.Flag)
				}
				if last.Flag != FlagComplete {
					t.Errorf("last frame Flag = %d, want FlagComplete (%d)", last.Flag, FlagComplete)
				}
			}
		})
	}
}

// TestWriteChunked_DataIntegrity verifies that the bytes reassembled
// from the chunked frames equal the original input.
func TestWriteChunked_DataIntegrity(t *testing.T) {
	sizes := []int{1, 100, chunkSize - 1, chunkSize, chunkSize + 1, chunkSize * 2, chunkSize*2 + 17, chunkSize * 5}
	for _, size := range sizes {
		t.Run("", func(t *testing.T) {
			want := bytes.Repeat([]byte{'Q'}, size)
			var buf bytes.Buffer
			ctx := &Context{wc: writeCloserAdapter{Writer: &buf}}
			if err := ctx.writeChunked(PacketTypeCommand, want); err != nil {
				t.Fatalf("writeChunked: %v", err)
			}

			var got []byte
			frames := readAllFrames(t, &buf)
			for _, f := range frames {
				got = append(got, f.Data...)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("reassembled data mismatch:\n got len=%d, want len=%d", len(got), len(want))
			}
		})
	}
}
