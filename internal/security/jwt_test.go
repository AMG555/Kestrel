package security

import (
	"testing"
	"time"
)

func TestSignAndVerifyJWT(t *testing.T) {
	secret := []byte("super-secret-key-1234567890123456")
	claims := JWTClaims{
		Subject:            "user-1",
		Username:           "alice",
		DisplayName:        "Alice",
		MustChangePassword: false,
		Roles:              []string{"operator"},
		Permissions:        map[string]bool{"tool:execute": true},
		PermissionScopes:   map[string]string{"tool:execute": "assigned"},
		Scope:              "assigned",
		ID:                 "token-123",
		IssuedAt:           time.Now().Unix(),
		ExpiresAt:          time.Now().Add(1 * time.Hour).Unix(),
	}

	token, err := SignJWT(claims, secret)
	if err != nil {
		t.Fatalf("failed to sign JWT: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	verified, err := VerifyJWT(token, secret)
	if err != nil {
		t.Fatalf("failed to verify valid JWT: %v", err)
	}
	if verified.Subject != claims.Subject {
		t.Errorf("subject mismatch: got %q, want %q", verified.Subject, claims.Subject)
	}
	if verified.Username != claims.Username {
		t.Errorf("username mismatch: got %q, want %q", verified.Username, claims.Username)
	}
	if !verified.Permissions["tool:execute"] {
		t.Error("expected tool:execute permission")
	}
}

func TestVerifyJWT_TamperedSignature(t *testing.T) {
	secret := []byte("super-secret-key-1234567890123456")
	wrongSecret := []byte("wrong-secret-key-1234567890123456")
	claims := JWTClaims{
		Subject:   "user-1",
		Username:  "alice",
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	}

	token, err := SignJWT(claims, secret)
	if err != nil {
		t.Fatalf("failed to sign JWT: %v", err)
	}

	_, err = VerifyJWT(token, wrongSecret)
	if err != ErrTokenSignature {
		t.Fatalf("expected ErrTokenSignature, got %v", err)
	}
}

func TestVerifyJWT_Expired(t *testing.T) {
	secret := []byte("super-secret-key-1234567890123456")
	claims := JWTClaims{
		Subject:   "user-1",
		Username:  "alice",
		ExpiresAt: time.Now().Add(-10 * time.Minute).Unix(),
	}

	token, err := SignJWT(claims, secret)
	if err != nil {
		t.Fatalf("failed to sign JWT: %v", err)
	}

	_, err = VerifyJWT(token, secret)
	if err != ErrTokenExpired {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestVerifyJWT_Malformed(t *testing.T) {
	secret := []byte("secret")
	for _, bad := range []string{"", "bad", "bad.token", "a.b.c.d"} {
		_, err := VerifyJWT(bad, secret)
		if err == nil {
			t.Errorf("expected error for malformed token %q", bad)
		}
	}
}

func TestAuthManager_SignedJWTValidation(t *testing.T) {
	mgr1 := NewAuthManager(12, "shared-cluster-secret-key-123456")
	mgr2 := NewAuthManager(12, "shared-cluster-secret-key-123456")

	claims := JWTClaims{
		Subject:            "user-admin",
		Username:           "admin",
		DisplayName:        "Administrator",
		MustChangePassword: false,
		Roles:              []string{"admin"},
		Permissions:        map[string]bool{"mcp:execute": true},
		ID:                 "jti-999",
		IssuedAt:           time.Now().Unix(),
		ExpiresAt:          time.Now().Add(2 * time.Hour).Unix(),
	}

	token, err := SignJWT(claims, mgr1.JWTSecret())
	if err != nil {
		t.Fatalf("SignJWT: %v", err)
	}

	// mgr2 has never seen this token in memory, but shares the secret
	session, ok := mgr2.ValidateToken(token)
	if !ok {
		t.Fatal("expected mgr2 to validate token from mgr1 statelessly")
	}
	if session.UserID != "user-admin" || session.Username != "admin" {
		t.Fatalf("unexpected session details: %+v", session)
	}
	if !session.Permissions["mcp:execute"] {
		t.Fatal("expected mcp:execute permission preserved in session")
	}

	// Test token revocation
	mgr2.RevokeToken(token)
	if _, ok := mgr2.ValidateToken(token); ok {
		t.Fatal("expected token to be rejected after revocation")
	}
}
