package models

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// TestLoginLog_BeforeCreate_HashesToken covers the security-relevant path:
// the raw JWT must never reach the database. SHA-256 hex is the wire
// format; the value is reproducible so we can pin the algorithm choice
// against future refactors.
func TestLoginLog_BeforeCreate_HashesToken(t *testing.T) {
	raw := "eyJhbGciOiJIUzI1NiJ9.payload.signature"
	log := &LoginLog{AccessToken: raw}
	if err := log.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if log.AccessToken == raw {
		t.Fatal("AccessToken must not be stored plaintext")
	}
	want := sha256Hex(raw)
	if log.AccessToken != want {
		t.Fatalf("hash mismatch: got %q want %q", log.AccessToken, want)
	}
}

// TestLoginLog_BeforeCreate_EmptyTokenPassesThrough covers the "no token
// in this log row" path: empty input must not produce a hash, otherwise
// the column would store a sentinel value that looks like a real token
// hash.
func TestLoginLog_BeforeCreate_EmptyTokenPassesThrough(t *testing.T) {
	log := &LoginLog{}
	if err := log.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if log.AccessToken != "" {
		t.Fatalf("empty token must stay empty, got %q", log.AccessToken)
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
