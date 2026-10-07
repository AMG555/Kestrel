package audit

import (
	"testing"
	"time"
)

func TestFailureThrottle(t *testing.T) {
	th := newFailureThrottle()
	key := "auth:login:127.0.0.1"
	cooldown := 100 * time.Millisecond

	// First attempt allowed
	if !th.allow(key, cooldown) {
		t.Fatalf("first attempt should be allowed")
	}

	// Immediate second attempt throttled
	if th.allow(key, cooldown) {
		t.Fatalf("immediate second attempt should be throttled")
	}

	// Different key allowed
	if !th.allow("auth:login:192.168.1.1", cooldown) {
		t.Fatalf("different key should be allowed")
	}

	// Wait past cooldown
	time.Sleep(120 * time.Millisecond)
	if !th.allow(key, cooldown) {
		t.Fatalf("attempt after cooldown should be allowed")
	}
}
