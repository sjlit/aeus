package models

import (
	"strings"
	"testing"

	"github.com/sjlit/aeus/pkg/errs"
)

// TestCheckPasswordPolicy pins the password baseline: 8-32 characters,
// alphanumeric only, at least one letter AND one digit. The regexp on
// User.Password's rule tag can only express length+charset (RE2 has no
// lookahead), so the letter+digit combination lives here — the hook and
// the services both call this function.
func TestCheckPasswordPolicy(t *testing.T) {
	cases := []struct {
		name    string
		pwd     string
		wantErr bool
	}{
		{"letters only", "abcdefgh", true},
		{"digits only", "12345678", true},
		{"too short", "a1b2c3d", true},
		{"too long", strings.Repeat("a1", 17), true}, // 34 chars
		{"space in middle", "abc 12345", true},
		{"symbol", "abc1234!", true},
		{"empty", "", true},
		{"min length valid", "a1b2c3d4", false},               // 8
		{"max length valid", strings.Repeat("a1", 16), false}, // 32
		{"mixed case valid", "AbcDef12", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckPasswordPolicy(tc.pwd)
			if !tc.wantErr {
				if err != nil {
					t.Fatalf("got %v, want nil", err)
				}
				return
			}
			if !errs.IsCode(err, errs.CodeInvalid) {
				t.Fatalf("got %v, want code %d", err, errs.CodeInvalid)
			}
		})
	}
}

// TestUser_BeforeCreate_WeakPlaintextRejected: the hook is the single
// choke point for every password write — REST create included — so a
// weak plaintext password must be rejected before hashing.
func TestUser_BeforeCreate_WeakPlaintextRejected(t *testing.T) {
	u := &User{Password: "weakpass"}
	err := u.BeforeCreate(nil)
	if !errs.IsCode(err, errs.CodeInvalid) {
		t.Fatalf("got %v, want code %d", err, errs.CodeInvalid)
	}
}

// TestUser_BeforeCreate_CompliantPlaintextHashed: policy-compliant
// plaintext passes through to bcrypt as before.
func TestUser_BeforeCreate_CompliantPlaintextHashed(t *testing.T) {
	u := &User{Password: "abc12345"}
	if err := u.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate: %v", err)
	}
	if !strings.HasPrefix(u.Password, "$2") {
		t.Fatalf("password not hashed: %q", u.Password)
	}
}

// TestUser_BeforeUpdate_HashedValueSkipped: updates that carry the
// already-hashed value (the common Save path) must not run the policy —
// a bcrypt hash is not an 8-32 alphanumeric plaintext.
func TestUser_BeforeUpdate_HashedValueSkipped(t *testing.T) {
	u := &User{Password: "$2a$10$abcdefghijklmnopqrstuvwxyz0123456789012345678901"}
	if err := u.BeforeUpdate(nil); err != nil {
		t.Fatalf("BeforeUpdate on hashed value: %v", err)
	}
}

// TestUser_BeforeCreate_EmptyPasswordAllowed: empty passwords stay
// allowed at the hook level (mirroring hashPassword's pass-through);
// the REST create path rejects them via the rule:"required" tag.
func TestUser_BeforeCreate_EmptyPasswordAllowed(t *testing.T) {
	u := &User{}
	if err := u.BeforeCreate(nil); err != nil {
		t.Fatalf("BeforeCreate on empty password: %v", err)
	}
}
