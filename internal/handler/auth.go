package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"kestrel/internal/auth"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	auth *auth.Service
	db   *database.DB
}

// NewAuthHandler creates an AuthHandler.
func NewAuthHandler(authSvc *auth.Service, db *database.DB) *AuthHandler {
	return &AuthHandler{auth: authSvc, db: db}
}

// loginRequest is the body for POST /api/auth/login.
type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login handles POST /api/auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	token, user, err := h.auth.Login(req.Username, req.Password, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		if err == auth.ErrBadCredentials {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":                token,
		"user_id":              user.ID,
		"username":             user.Username,
		"display_name":         user.DisplayName,
		"must_change_password": user.MustChangePassword,
	})
}

// Logout handles POST /api/auth/logout.
func (h *AuthHandler) Logout(c *gin.Context) {
	token := auth.ExtractBearerToken(c.Request)
	if token != "" {
		_ = h.auth.Logout(token)
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

// changePasswordRequest is the body for POST /api/auth/change-password.
type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// ChangePassword handles POST /api/auth/change-password.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID, _ := c.Get(middleware.CtxUserID)
	uid, _ := userID.(string)

	var req changePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current_password and new_password are required"})
		return
	}

	if len(req.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new password must be at least 8 characters"})
		return
	}

	user, err := h.db.GetUserByID(uid)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}

	if !auth.CheckPassword(req.CurrentPassword, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	if err := h.db.UpdateUserPassword(uid, newHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "password changed successfully"})
}

// Me handles GET /api/auth/me.
func (h *AuthHandler) Me(c *gin.Context) {
	userID, _ := c.Get(middleware.CtxUserID)
	uid, _ := userID.(string)

	user, err := h.db.GetUserByID(uid)
	if err != nil || user == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}

	roles, err := h.db.GetUserRoles(uid)
	if err != nil {
		roles = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                   user.ID,
		"username":             user.Username,
		"display_name":         user.DisplayName,
		"email":                user.Email,
		"must_change_password": user.MustChangePassword,
		"roles":                roles,
	})
}
