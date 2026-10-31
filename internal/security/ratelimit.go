package security

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// rateLimitEntry records the request window info for an IP address or key.
type rateLimitEntry struct {
	count    int
	windowAt time.Time
}

// RateLimiter is a sliding-window rate limiter keyed by string (IP, username, etc.).
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateLimitEntry
	limit   int           // maximum requests allowed within the window
	window  time.Duration // window duration
}

// NewRateLimiter creates a rate limiter.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		entries: make(map[string]*rateLimitEntry),
		limit:   limit,
		window:  window,
	}
	// Periodically clean up expired entries in the background to prevent memory leaks.
	go rl.cleanup()
	return rl
}

// cleanup removes expired entries once per minute.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for key, entry := range rl.entries {
			if now.Sub(entry.windowAt) > rl.window {
				delete(rl.entries, key)
			}
		}
		rl.mu.Unlock()
	}
}

// Allow checks whether the specified key (IP, username, etc.) is allowed through.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, ok := rl.entries[key]
	if !ok || now.Sub(entry.windowAt) > rl.window {
		rl.entries[key] = &rateLimitEntry{count: 1, windowAt: now}
		return true
	}

	entry.count++
	return entry.count <= rl.limit
}

func (rl *RateLimiter) allow(ip string) bool {
	return rl.Allow(ip)
}

// RateLimitMiddleware returns a Gin middleware that responds with 429 for requests that exceed the rate limit.
func RateLimitMiddleware(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.Allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded, please try again later",
			})
			return
		}
		c.Next()
	}
}
