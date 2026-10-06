package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// SecurityHeaders adds standard defensive HTTP security headers to every response.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy",
			"default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:")
		// HSTS is only sent over HTTPS.
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// CORS adds permissive CORS headers for the configured origins.
// Pass nil or an empty slice to allow all origins (development only).
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowAll := len(allowedOrigins) == 0
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = struct{}{}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowAll || origin == "" {
			if origin == "" {
				origin = "*"
			}
			c.Header("Access-Control-Allow-Origin", origin)
		} else if _, ok := allowed[origin]; ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// ── Rate limiter (token-bucket per IP key) ───────────────────────────────────

type bucket struct {
	count    int
	windowAt time.Time
}

// RateLimiter is a simple sliding-window per-key rate limiter.
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*bucket
	maxPerWin  int
	windowDur  time.Duration
}

// NewRateLimiter creates a rate limiter allowing maxPerWindow requests per window.
func NewRateLimiter(maxPerWindow, windowSecs int) *RateLimiter {
	if maxPerWindow <= 0 {
		maxPerWindow = 10
	}
	if windowSecs <= 0 {
		windowSecs = 60
	}
	return &RateLimiter{
		buckets:   make(map[string]*bucket),
		maxPerWin: maxPerWindow,
		windowDur: time.Duration(windowSecs) * time.Second,
	}
}

// Allow returns true if the key is within its rate limit for the current window.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[key]
	if !ok || now.After(b.windowAt.Add(rl.windowDur)) {
		rl.buckets[key] = &bucket{count: 1, windowAt: now}
		return true
	}
	if b.count >= rl.maxPerWin {
		return false
	}
	b.count++
	return true
}

// Purge removes stale bucket entries. Call periodically to prevent unbounded growth.
func (rl *RateLimiter) Purge() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	for k, b := range rl.buckets {
		if now.After(b.windowAt.Add(rl.windowDur * 2)) {
			delete(rl.buckets, k)
		}
	}
}

// LoginRateLimit returns a Gin middleware that rate-limits login attempts per IP.
func LoginRateLimit(rl *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if !rl.Allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "too many login attempts — please wait before retrying",
			})
			return
		}
		c.Next()
	}
}
