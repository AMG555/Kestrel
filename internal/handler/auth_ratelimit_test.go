package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kestrel/internal/config"
	"kestrel/internal/security"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestAuthHandler_LoginUsernameRateLimiting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	authMgr := security.NewAuthManager(1)
	authHandler := NewAuthHandler(authMgr, &config.Config{}, "", logger)
	// Override limiter to small threshold for test
	authHandler.userLimiter = security.NewRateLimiter(2, 100*time.Millisecond)

	r := gin.New()
	r.POST("/api/auth/login", authHandler.Login)

	loginBody := func(user string) *bytes.Buffer {
		b, _ := json.Marshal(map[string]string{"username": user, "password": "wrongpassword"})
		return bytes.NewBuffer(b)
	}

	// First attempt
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody("targetuser"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Fatalf("expected first attempt not to be rate limited, got 429")
	}

	// Second attempt
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody("targetuser"))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Fatalf("expected second attempt not to be rate limited, got 429")
	}

	// Third attempt -> should be 429
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody("targetuser"))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected third attempt to be 429 Too Many Requests, got %d", w.Code)
	}

	// Different username should not be throttled
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody("otheruser"))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusTooManyRequests {
		t.Fatalf("expected different username not to be rate limited, got 429")
	}
}

