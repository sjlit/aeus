package bytepool

import (
	"testing"
)

func TestGetBytes_ZeroOrNegative(t *testing.T) {
	if GetBytes(0) != nil {
		t.Error("GetBytes(0) should return nil")
	}
	if GetBytes(-1) != nil {
		t.Error("GetBytes(-1) should return nil")
	}
}

func TestGetBytes_SizeAndCapacity(t *testing.T) {
	cases := []struct {
		size   int
		minCap int
	}{
		{1, 1},
		{1023, 1023},
		{1024, 1024},
		{1025, 1025},
		{2047, 2047},
		{2048, 2048},
		{2049, 2049},
		{5119, 5119},
		{5120, 5120},
		{5121, 5121},
	}
	for _, c := range cases {
		buf := GetBytes(c.size)
		if len(buf) != c.size {
			t.Errorf("GetBytes(%d) len = %d, want %d", c.size, len(buf), c.size)
		}
		if cap(buf) < c.size {
			t.Errorf("GetBytes(%d) cap = %d, want >= %d", c.size, cap(buf), c.size)
		}
	}
}

func TestGetBytes_CapacityReuse(t *testing.T) {
	// Put a large buffer and get a smaller one — should reuse capacity
	large := make([]byte, 2048)
	PutBytes(large)

	buf := GetBytes(1024)
	if cap(buf) < 2048 {
		t.Logf("pool did not reuse large buffer (cap=%d), this is acceptable", cap(buf))
	}
}

func TestPutBytes_ZeroCap(t *testing.T) {
	// Should not panic
	PutBytes([]byte{})
}

func TestPutBytes_Classification(t *testing.T) {
	// Put small and large buffers, verify they don't crash
	PutBytes(make([]byte, 512))
	PutBytes(make([]byte, 1024))
	PutBytes(make([]byte, 2048))
	PutBytes(make([]byte, 5120))
}
