package audit

import (
	"sync"
	"time"
)

// failureThrottle deduplicates high-frequency failure audit records (e.g. repeated bad login attempts).
type failureThrottle struct {
	mu   sync.Mutex
	last map[string]time.Time
}

func newFailureThrottle() *failureThrottle {
	return &failureThrottle{last: make(map[string]time.Time)}
}

// allow reports whether a record with the given key may be written now.
func (t *failureThrottle) allow(key string, cooldown time.Duration) bool {
	if t == nil || cooldown <= 0 || key == "" {
		return true
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	if prev, ok := t.last[key]; ok && now.Sub(prev) < cooldown {
		return false
	}
	t.last[key] = now
	if len(t.last) > 4096 {
		for k, ts := range t.last {
			if now.Sub(ts) > cooldown*2 {
				delete(t.last, k)
			}
		}
	}
	return true
}

// AuthFailureThrottleKey builds a per-IP key for auth failure deduplication.
func AuthFailureThrottleKey(category, action, clientIP string) string {
	return category + ":" + action + ":" + clientIP
}

// IsAuthFailureThrottled checks if a given action qualifies for auth failure throttling.
func IsAuthFailureThrottled(category, action string) bool {
	if category != CategoryAuth {
		return false
	}
	switch action {
	case ActionLogin, ActionChangePassword:
		return true
	default:
		return false
	}
}
