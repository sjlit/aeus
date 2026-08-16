package netutil

import (
	"net"
	"testing"
)

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

type fakeListener struct {
	addr fakeAddr
}

func (f *fakeListener) Accept() (net.Conn, error) { return nil, nil }
func (f *fakeListener) Close() error              { return nil }
func (f *fakeListener) Addr() net.Addr            { return f.addr }

func TestEffectiveAddr_WithHost(t *testing.T) {
	got := EffectiveAddr("127.0.0.1:8080", nil)
	if got != "127.0.0.1:8080" {
		t.Errorf("EffectiveAddr = %q, want \"127.0.0.1:8080\"", got)
	}
}

func TestEffectiveAddr_WithZeroIP(t *testing.T) {
	l := &fakeListener{addr: "192.168.1.1:54321"}
	got := EffectiveAddr("0.0.0.0:0", l)
	if got != "192.168.1.1:54321" {
		t.Errorf("EffectiveAddr = %q, want \"192.168.1.1:54321\"", got)
	}
}

func TestEffectiveAddr_WithIPv6(t *testing.T) {
	l := &fakeListener{addr: "192.168.1.1:54321"}
	got := EffectiveAddr("[::]:8080", l)
	if got != "192.168.1.1:54321" {
		t.Errorf("EffectiveAddr = %q, want \"192.168.1.1:54321\"", got)
	}
}

func TestEffectiveAddr_ListenerOverridesPort(t *testing.T) {
	l := &fakeListener{addr: "127.0.0.1:54321"}
	got := EffectiveAddr(":0", l)
	if got != "127.0.0.1:54321" {
		t.Errorf("EffectiveAddr = %q, want \"127.0.0.1:54321\"", got)
	}
}

func TestEffectiveAddr_InvalidAddrAndNilListener(t *testing.T) {
	got := EffectiveAddr("not-an-addr", nil)
	if got != "" {
		t.Errorf("EffectiveAddr = %q, want empty string", got)
	}
}

func TestEffectiveAddr_LocalIPFallback(t *testing.T) {
	l := &fakeListener{addr: "0.0.0.0:8080"}
	got := EffectiveAddr("0.0.0.0:0", l)
	if got == "" {
		t.Error("EffectiveAddr should not return empty when listener provides port")
	}
	if got == "0.0.0.0:8080" {
		t.Error("EffectiveAddr should fallback to LocalIP when listener returns 0.0.0.0")
	}
}
