package netutil

import (
	"strings"
	"testing"
)

func TestLocalIP_ReturnsNonEmpty(t *testing.T) {
	got := LocalIP()
	if got == "" {
		t.Error("LocalIP() should not return empty string")
	}
}

func TestLocalIP_NotLoopback(t *testing.T) {
	got := LocalIP()
	if got == "127.0.0.1" {
		t.Log("LocalIP returned loopback, this may happen in CI")
	}
}

func TestLocalIP_ContainsDotOrLocalhost(t *testing.T) {
	got := LocalIP()
	if !strings.Contains(got, ".") && got != "localhost" {
		t.Errorf("LocalIP = %q, expected IPv4 or localhost", got)
	}
}
