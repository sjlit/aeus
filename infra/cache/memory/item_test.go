package memory

import (
	"testing"
	"time"
)

func TestItemExpired_NeverExpire(t *testing.T) {
	item := Item{Value: "data", Expiration: 0}
	if item.Expired() {
		t.Error("Expiration=0 should never expire")
	}
}

func TestItemExpired_NotYet(t *testing.T) {
	item := Item{Value: "data", Expiration: time.Now().Add(time.Hour).UnixNano()}
	if item.Expired() {
		t.Error("future expiration should not be expired")
	}
}

func TestItemExpired_Already(t *testing.T) {
	item := Item{Value: "data", Expiration: time.Now().Add(-time.Hour).UnixNano()}
	if !item.Expired() {
		t.Error("past expiration should be expired")
	}
}

func TestItemExpired_ZeroValue(t *testing.T) {
	var item Item
	if item.Expired() {
		t.Error("zero-value Item should not be expired")
	}
}
