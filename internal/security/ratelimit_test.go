package security

import (
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(3, 50*time.Millisecond)

	// First 3 requests should be allowed
	if !rl.Allow("user1") {
		t.Fatalf("expected request 1 to be allowed")
	}
	if !rl.Allow("user1") {
		t.Fatalf("expected request 2 to be allowed")
	}
	if !rl.Allow("user1") {
		t.Fatalf("expected request 3 to be allowed")
	}

	// 4th request within window should be rejected
	if rl.Allow("user1") {
		t.Fatalf("expected request 4 to be rejected")
	}

	// Different key should still be allowed
	if !rl.Allow("user2") {
		t.Fatalf("expected request for user2 to be allowed")
	}

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	// After window expiry, request should be allowed again
	if !rl.Allow("user1") {
		t.Fatalf("expected request after window expiry to be allowed")
	}
}

