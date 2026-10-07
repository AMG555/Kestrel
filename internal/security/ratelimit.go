package security

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// rateLimitEntry records the request window info for an IP address.
type rateLimitEntry struct {
	count    int
	windowAt time.Time
}

// RateLimiter is an IP-based sliding-window rate limiter.
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
		for ip, entry := range rl.entries {
			if now.Sub(entry.windowAt) > rl.window {
				delete(rl.entries, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// allow checks whether the specified IP is allowed through.
func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, ok := rl.entries[ip]
	if !ok || now.Sub(entry.windowAt) > rl.window {
		rl.entries[ip] = &rateLimitEntry{count: 1, windowAt: now}
		return true
	}

	entry.count++
	return entry.count <= rl.limit
}

// RateLimitMiddleware returns a Gin middleware that responds with 429 for requests that exceed the rate limit.
func RateLimitMiddleware(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !rl.allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded, please try again later",
			})
			return
		}
		c.Next()
	}
}
