package cli

import (
	"bytes"
	"errors"
	"math"
	"strconv"
	"testing"
)

// TestReadFrame_DataAndError verifies that a frame carrying both a data
// section and an error section decodes correctly, even when the error
// string is shorter than the data. The original implementation compared
// the bytes read for the error section against dataLength, so such
// frames failed with io.ErrShortBuffer despite being well-formed.
func TestReadFrame_DataAndError(t *testing.T) {
	var buf bytes.Buffer
	f := newFrame(PacketTypeCommand, FlagComplete, 7, 0, []byte("data"))
	f.Error = "E"
	if err := writeFrame(&buf, f); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}
	got, err := readFrame(&buf)
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if !bytes.Equal(got.Data, f.Data) {
		t.Errorf("Data = %q, want %q", got.Data, f.Data)
	}
	if got.Error != f.Error {
		t.Errorf("Error = %q, want %q", got.Error, f.Error)
	}
	if got.Seq != 7 || got.Type != PacketTypeCommand || got.Flag != FlagComplete {
		t.Errorf("header mismatch: type=%d flag=%d seq=%d", got.Type, got.Flag, got.Seq)
	}
}

// TestReadFrame_TruncatedData verifies a mid-frame stream failure surfaces
// as a read error (and is not masked by a subsequent section read).
func TestReadFrame_TruncatedData(t *testing.T) {
	var buf bytes.Buffer
	f := newFrame(PacketTypeCommand, FlagComplete, 1, 0, bytes.Repeat([]byte("D"), 64))
	if err := writeFrame(&buf, f); err != nil {
		t.Fatal(err)
	}
	full := buf.Bytes()
	if _, err := readFrame(bytes.NewReader(full[:len(full)-1])); err == nil {
		t.Error("expected error on truncated frame, got nil")
	}
}

// TestSerializeUnsigned verifies that unsigned integers serialize to their
// decimal form instead of panicking. The original implementation matched
// uint kinds in its switch but called reflect.Value.Int on them, which
// panics; uint fell through to JSON by accident.
func TestSerializeUnsigned(t *testing.T) {
	cases := []struct {
		val  any
		want string
	}{
		{uint(42), "42"},
		{uint8(7), "7"},
		{uint16(65535), "65535"},
		{uint32(4000000000), "4000000000"},
		{uint64(math.MaxUint64), strconv.FormatUint(math.MaxUint64, 10)},
		{int(-5), "-5"},
		{int64(math.MinInt64), strconv.FormatInt(math.MinInt64, 10)},
	}
	for _, tc := range cases {
		got, err := serialize(tc.val)
		if err != nil {
			t.Errorf("%T: serialize: %v", tc.val, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%T: serialize = %q, want %q", tc.val, got, tc.want)
		}
	}
}

// TestRouter_DuplicateHandleReturnsError verifies duplicate registration
// returns ErrHandleRegistered rather than panicking.
func TestRouter_DuplicateHandleReturnsError(t *testing.T) {
	r := newRouter("")
	cmd := Command{Handle: func(*Context) error { return nil }}
	if err := r.Handle("/help", cmd); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	err := r.Handle("/help", cmd)
	if !errors.Is(err, ErrHandleRegistered) {
		t.Fatalf("second Handle: err = %v, want ErrHandleRegistered", err)
	}
}

// TestNewRegistersHelp verifies /help is registered in New (not Start), so
// calling Start again after a restart does not fail with a duplicate error.
func TestNewRegistersHelp(t *testing.T) {
	t.Setenv("CLI_PORT", "")
	svr := New()
	router, _, err := svr.router.Lookup([]string{"help"})
	if err != nil {
		t.Fatalf("Lookup help: %v", err)
	}
	if router.command.Handle == nil {
		t.Error("/help has no handler")
	}
}
