package audit

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// ── SanitizeDetail ────────────────────────────────────────────────────────────

func TestSanitizeDetailRedactsSensitiveKeys(t *testing.T) {
	detail := map[string]interface{}{
		"username":    "alice",
		"password":    "s3cr3t",
		"api_key":     "key-abc123",
		"secret":      "topsecret",
		"token":       "jwt.xxx.yyy",
		"description": "normal value",
	}
	got := SanitizeDetail(detail, 8192)

	for _, key := range []string{"password", "api_key", "secret", "token"} {
		if got[key] != "***" {
			t.Errorf("key %q: expected *** got %v", key, got[key])
		}
	}
	if got["username"] != "alice" {
		t.Errorf("username should not be redacted, got %v", got["username"])
	}
	if got["description"] != "normal value" {
		t.Errorf("description should not be redacted, got %v", got["description"])
	}
}

func TestSanitizeDetailRedactsNestedSensitiveKeys(t *testing.T) {
	detail := map[string]interface{}{
		"config": map[string]interface{}{
			"api_key":  "nested-key",
			"endpoint": "https://example.com",
		},
	}
	got := SanitizeDetail(detail, 8192)
	inner, ok := got["config"].(map[string]interface{})
	if !ok {
		t.Fatal("inner config should be a map")
	}
	if inner["api_key"] != "***" {
		t.Errorf("nested api_key should be redacted, got %v", inner["api_key"])
	}
	if inner["endpoint"] != "https://example.com" {
		t.Errorf("endpoint should not be redacted, got %v", inner["endpoint"])
	}
}

func TestSanitizeDetailTruncatesOversizedPayload(t *testing.T) {
	// Build a detail that serialises to more than maxBytes
	big := strings.Repeat("x", 200)
	detail := map[string]interface{}{}
	for i := 0; i < 60; i++ {
		detail[big] = big
	}
	got := SanitizeDetail(detail, 256)
	if got["_truncated"] != true {
		t.Errorf("oversized payload should have _truncated=true, got %v", got)
	}
	if _, ok := got["_preview"]; !ok {
		t.Error("_preview key expected in truncated detail")
	}
}

func TestSanitizeDetailNilReturnsNil(t *testing.T) {
	if SanitizeDetail(nil, 8192) != nil {
		t.Error("nil detail should return nil")
	}
}

func TestSanitizeDetailZeroMaxBytesUsesDefault(t *testing.T) {
	detail := map[string]interface{}{"k": "v"}
	got := SanitizeDetail(detail, 0)
	if got["k"] != "v" {
		t.Errorf("zero maxBytes should use default, got %v", got)
	}
}

// ── HintFromToken / sessionHint ───────────────────────────────────────────────

func TestHintFromTokenIsDeterministic(t *testing.T) {
	h1 := HintFromToken("tok-abc")
	h2 := HintFromToken("tok-abc")
	if h1 != h2 {
		t.Errorf("hint is not deterministic: %q vs %q", h1, h2)
	}
}

func TestHintFromTokenDifferentInputsDifferentHints(t *testing.T) {
	h1 := HintFromToken("tokenA")
	h2 := HintFromToken("tokenB")
	if h1 == h2 {
		t.Errorf("different tokens should produce different hints")
	}
}

func TestHintFromTokenEmptyReturnsEmpty(t *testing.T) {
	if h := HintFromToken(""); h != "" {
		t.Errorf("empty token should return empty hint, got %q", h)
	}
}

func TestHintFromTokenHasExpectedLength(t *testing.T) {
	h := HintFromToken("any-token-value")
	// SHA-256 first 4 bytes = 8 hex chars
	if len(h) != 8 {
		t.Errorf("hint should be 8 hex chars, got %q (len %d)", h, len(h))
	}
}

// ── failureThrottle ───────────────────────────────────────────────────────────

func TestFailureThrottleAllowsFirstCall(t *testing.T) {
	ft := newFailureThrottle()
	if !ft.allow("key1", time.Second) {
		t.Error("first call should be allowed")
	}
}

func TestFailureThrottleBlocksWithinCooldown(t *testing.T) {
	ft := newFailureThrottle()
	ft.allow("key1", time.Minute) // prime
	if ft.allow("key1", time.Minute) {
		t.Error("second call within cooldown should be blocked")
	}
}

func TestFailureThrottleAllowsAfterCooldown(t *testing.T) {
	ft := newFailureThrottle()
	ft.allow("key1", time.Nanosecond) // prime with tiny cooldown
	time.Sleep(2 * time.Millisecond)
	if !ft.allow("key1", time.Nanosecond) {
		t.Error("call after cooldown expiry should be allowed")
	}
}

func TestFailureThrottleIsConcurrentlySafe(t *testing.T) {
	ft := newFailureThrottle()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ft.allow("key-concurrent", time.Millisecond)
		}()
	}
	wg.Wait()
}

func TestFailureThrottleNilIsAlwaysAllowed(t *testing.T) {
	var ft *failureThrottle
	if !ft.allow("any", time.Second) {
		t.Error("nil throttle should always allow")
	}
}

// ── isAuthFailureThrottled ────────────────────────────────────────────────────

func TestIsAuthFailureThrottledOnlyForAuthCategory(t *testing.T) {
	cases := []struct {
		category, action string
		want             bool
	}{
		{"auth", "login", true},
		{"auth", "change_password", true},
		{"auth", "logout", false},
		{"config", "login", false},
		{"audit", "login", false},
	}
	for _, tc := range cases {
		got := isAuthFailureThrottled(tc.category, tc.action)
		if got != tc.want {
			t.Errorf("isAuthFailureThrottled(%q,%q) = %v, want %v", tc.category, tc.action, got, tc.want)
		}
	}
}

// ── authFailureThrottleKey ────────────────────────────────────────────────────

func TestAuthFailureThrottleKeyIncludesAllComponents(t *testing.T) {
	key := authFailureThrottleKey("auth", "login", "1.2.3.4")
	if !strings.Contains(key, "auth") || !strings.Contains(key, "login") || !strings.Contains(key, "1.2.3.4") {
		t.Errorf("key missing components: %q", key)
	}
}

// ── Service nil-safety ────────────────────────────────────────────────────────

func TestServiceNilDoesNotPanic(t *testing.T) {
	var s *Service
	// None of these should panic
	s.Record(nil, Entry{Category: "auth", Action: "login"})
	s.RecordSystem(Entry{Category: "auth", Action: "login"})
	s.RecordOK(nil, "auth", "login", "msg", "conversation", "id-1", nil)
	s.RecordFail(nil, "auth", "login", "msg", nil)
	s.PurgeExpired()
	if s.Enabled() {
		t.Error("nil service should not be enabled")
	}
	if s.RetentionDays() != 0 {
		t.Error("nil service RetentionDays should be 0")
	}
}
