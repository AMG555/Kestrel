package auth_test

import (
	"testing"

	"kestrel/internal/auth"
)

func TestHashAndVerify(t *testing.T) {
	password := "S3cur3P@ssw0rd!"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error: %v", err)
	}
	if hash == "" {
		t.Fatal("hash must not be empty")
	}
	if hash == password {
		t.Fatal("hash must not equal plaintext")
	}

	if !auth.CheckPassword(password, hash) {
		t.Error("CheckPassword(correct) returned false")
	}
	if auth.CheckPassword("wrongpassword", hash) {
		t.Error("CheckPassword(wrong) returned true")
	}
}

func TestHashUniqueness(t *testing.T) {
	// Same password should produce different hashes (bcrypt salt).
	password := "samepassword"
	h1, _ := auth.HashPassword(password)
	h2, _ := auth.HashPassword(password)
	if h1 == h2 {
		t.Error("two hashes of the same password should differ (different salts)")
	}
}

func TestJWTRoundtrip(t *testing.T) {
	secret := "test-jwt-secret-32-bytes-exactly!"
	svc := auth.NewForTest(secret)

	token, err := svc.IssueToken("user-123", "admin-session-1")
	if err != nil {
		t.Fatalf("IssueToken() error: %v", err)
	}
	if token == "" {
		t.Fatal("token must not be empty")
	}

	claims, err := svc.ValidateTokenInsecure(token)
	if err != nil {
		t.Fatalf("ValidateToken() error: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-123")
	}
	if claims.SessionID != "admin-session-1" {
		t.Errorf("SessionID = %q, want %q", claims.SessionID, "admin-session-1")
	}
}

func TestJWTInvalidToken(t *testing.T) {
	svc := auth.NewForTest("any-secret-value-here")
	_, err := svc.ValidateTokenInsecure("not.a.valid.token")
	if err == nil {
		t.Error("ValidateToken(invalid) should return error")
	}
}
