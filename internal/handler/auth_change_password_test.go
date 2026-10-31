package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"kestrel/internal/config"
	"kestrel/internal/security"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestAuthHandler_ChangePasswordContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	logger := zap.NewNop()
	authMgr := security.NewAuthManager(1)
	authHandler := NewAuthHandler(authMgr, &config.Config{}, "", logger)

	r := gin.New()
	r.POST("/api/auth/change-password", authHandler.ChangePassword)

	// Test snake_case payload from React SPA: { current_password, new_password }
	body, _ := json.Marshal(map[string]string{
		"current_password": "currentPassword123",
		"new_password":     "newPassword456 with spaces",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["error"] == "current password and new password cannot be empty" {
		t.Fatalf("snake_case payload failed binding: %v", resp)
	}

	// Verify that password with valid length passed length validation
	if resp["error"] == "new password must be at least 8 characters" {
		t.Fatalf("unexpected length error: %v", resp)
	}
}
