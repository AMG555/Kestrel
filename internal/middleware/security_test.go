package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"kestrel/internal/middleware"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ── SecurityHeaders ───────────────────────────────────────────────────────────

func TestSecurityHeadersPresent(t *testing.T) {
	router := gin.New()
	router.Use(middleware.SecurityHeaders())
	router.GET("/ping", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	router.ServeHTTP(w, req)

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"X-XSS-Protection":       "1; mode=block",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, val := range want {
		if got := w.Header().Get(header); got != val {
			t.Errorf("%s = %q, want %q", header, got, val)
		}
	}
	if csp := w.Header().Get("Content-Security-Policy"); csp == "" {
		t.Error("Content-Security-Policy header missing")
	}
}

func TestHSTSOnlyOverHTTPS(t *testing.T) {
	router := gin.New()
	router.Use(middleware.SecurityHeaders())
	router.GET("/ping", func(c *gin.Context) { c.Status(200) })

	// Plain HTTP — no HSTS.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)
	router.ServeHTTP(w, req)
	if w.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS should not be set on plain HTTP")
	}
}

// ── CORS ──────────────────────────────────────────────────────────────────────

func TestCORSAllowsOrigin(t *testing.T) {
	router := gin.New()
	router.Use(middleware.CORS([]string{"https://app.example.com"}))
	router.GET("/api", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://app.example.com")
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("ACAO = %q, want https://app.example.com", got)
	}
}

func TestCORSBlocksUnknownOrigin(t *testing.T) {
	router := gin.New()
	router.Use(middleware.CORS([]string{"https://app.example.com"}))
	router.GET("/api", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	router.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO should be empty for unknown origin, got %q", got)
	}
}

func TestCORSPreflightReturns204(t *testing.T) {
	router := gin.New()
	router.Use(middleware.CORS(nil)) // allow all
	router.OPTIONS("/api", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodOptions, "/api", nil)
	req.Header.Set("Origin", "https://anything.example.com")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", w.Code)
	}
}

// ── Rate limiter ──────────────────────────────────────────────────────────────

func TestRateLimiterAllowsUnderLimit(t *testing.T) {
	rl := middleware.NewRateLimiter(5, 60)
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.0.2.1") {
			t.Errorf("attempt %d should be allowed", i+1)
		}
	}
}

func TestRateLimiterBlocksOverLimit(t *testing.T) {
	rl := middleware.NewRateLimiter(3, 60)
	for i := 0; i < 3; i++ {
		rl.Allow("10.0.0.1")
	}
	if rl.Allow("10.0.0.1") {
		t.Error("4th attempt should be blocked")
	}
}

func TestRateLimiterIndependentKeys(t *testing.T) {
	rl := middleware.NewRateLimiter(2, 60)
	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.1")
	// Second IP should still be allowed.
	if !rl.Allow("10.0.0.2") {
		t.Error("different IP should have its own bucket")
	}
}

func TestRateLimiterPurge(t *testing.T) {
	rl := middleware.NewRateLimiter(2, 1) // 1-second window
	rl.Allow("10.0.0.99")
	time.Sleep(3 * time.Millisecond)
	rl.Purge() // should not panic
}

func TestLoginRateLimitMiddleware(t *testing.T) {
	rl := middleware.NewRateLimiter(2, 60)
	router := gin.New()
	router.POST("/auth/login", middleware.LoginRateLimit(rl), func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	attempt := func() int {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodPost, "/auth/login",
			strings.NewReader(`{"username":"a","password":"b"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		router.ServeHTTP(w, req)
		return w.Code
	}

	if c := attempt(); c != 200 { t.Errorf("attempt 1 got %d, want 200", c) }
	if c := attempt(); c != 200 { t.Errorf("attempt 2 got %d, want 200", c) }
	if c := attempt(); c != http.StatusTooManyRequests {
		t.Errorf("attempt 3 got %d, want 429", c)
	}
}
