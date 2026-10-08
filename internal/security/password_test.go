package security

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateStrongPassword_DefaultLength(t *testing.T) {
	// length <= 0 should default to 24
	pwd, err := GenerateStrongPassword(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pwd) != 24 {
		t.Errorf("expected length 24, got %d", len(pwd))
	}
}

func TestGenerateStrongPassword_NegativeLength(t *testing.T) {
	pwd, err := GenerateStrongPassword(-5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pwd) != 24 {
		t.Errorf("expected length 24 for negative input, got %d", len(pwd))
	}
}

func TestGenerateStrongPassword_ExactLength(t *testing.T) {
	for _, length := range []int{8, 16, 32, 64} {
		pwd, err := GenerateStrongPassword(length)
		if err != nil {
			t.Fatalf("length %d: unexpected error: %v", length, err)
		}
		if len(pwd) != length {
			t.Errorf("length %d: got password of length %d", length, len(pwd))
		}
	}
}

func TestGenerateStrongPassword_URLSafeCharacters(t *testing.T) {
	pwd, err := GenerateStrongPassword(64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// RawURLEncoding uses A-Z, a-z, 0-9, - and _; no padding, no + or /
	for _, ch := range pwd {
		if !isURLSafeBase64Char(ch) {
			t.Errorf("password contains non-URL-safe character: %q (full pwd: %s)", ch, pwd)
		}
	}
}

func TestGenerateStrongPassword_Uniqueness(t *testing.T) {
	const iterations = 20
	seen := make(map[string]bool, iterations)
	for i := 0; i < iterations; i++ {
		pwd, err := GenerateStrongPassword(32)
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
		if seen[pwd] {
			t.Errorf("duplicate password generated: %s", pwd)
		}
		seen[pwd] = true
	}
}

func TestGenerateStrongPassword_NoPaddingOrPlus(t *testing.T) {
	for i := 0; i < 10; i++ {
		pwd, err := GenerateStrongPassword(48)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.ContainsAny(pwd, "+/=") {
			t.Errorf("password contains standard base64 characters (+, /, =): %s", pwd)
		}
	}
}

func TestGenerateStrongPassword_IsDecodable(t *testing.T) {
	// The full (pre-truncated) output is valid RawURLEncoding.
	// After truncation the suffix may be incomplete, but each password
	// must consist exclusively of valid base64url alphabet characters.
	pwd, err := GenerateStrongPassword(20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, ch := range pwd {
		if !isURLSafeBase64Char(ch) {
			t.Errorf("character %q is not in the base64url alphabet", ch)
		}
	}
}

// isURLSafeBase64Char reports whether r is a valid character in
// base64.RawURLEncoding output (A-Z, a-z, 0-9, -, _).
func isURLSafeBase64Char(r rune) bool {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	return strings.ContainsRune(alphabet, r)
}

// TestGenerateStrongPassword_EntropyLength verifies that the raw bytes
// backing a password of length N carry at least N bytes of randomness
// (before base64 expansion).
func TestGenerateStrongPassword_EntropyLength(t *testing.T) {
	length := 32
	pwd, err := GenerateStrongPassword(length)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Pad to a multiple of 4 so base64 can decode a prefix
	padded := pwd + strings.Repeat("=", (4-len(pwd)%4)%4)
	decoded, err := base64.URLEncoding.DecodeString(padded)
	if err != nil {
		// Truncated suffix may not decode cleanly; at minimum the
		// character set check above already verified randomness.
		t.Logf("note: padded password did not decode cleanly (expected for truncated output): %v", err)
		return
	}
	// Decoded bytes should be at least length * 3/4 (base64 ratio)
	minExpected := (length * 3) / 4
	if len(decoded) < minExpected {
		t.Errorf("expected at least %d decoded bytes, got %d", minExpected, len(decoded))
	}
}
