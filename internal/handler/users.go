package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
)

// UserHandler handles user management endpoints.
type UserHandler struct {
	db *database.DB
}

// NewUserHandler creates a UserHandler.
func NewUserHandler(db *database.DB) *UserHandler {
	return &UserHandler{db: db}
}

// ListUsers handles GET /api/users.
func (h *UserHandler) ListUsers(c *gin.Context) {
	users, err := h.db.ListUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list users"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users, "total": len(users)})
}

// createUserRequest is the body for POST /api/users.
type createUserRequest struct {
	Username    string `json:"username" binding:"required"`
	Password    string `json:"password" binding:"required"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// CreateUser handles POST /api/users.
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}
	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	user, err := h.db.CreateUser(req.Username, hash, req.Email, req.DisplayName)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "username already exists or database error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": user})
}

// GetUser handles GET /api/users/:id.
func (h *UserHandler) GetUser(c *gin.Context) {
	id := c.Param("id")
	user, err := h.db.GetUserByID(id)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
}

// updateUserRequest is the body for PATCH /api/users/:id.
type updateUserRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	IsActive    *bool  `json:"is_active"`
}

// UpdateUser handles PATCH /api/users/:id.
func (h *UserHandler) UpdateUser(c *gin.Context) {
	id := c.Param("id")
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := h.db.GetUserByID(id)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	isActive := user.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	displayName := user.DisplayName
	if req.DisplayName != "" {
		displayName = req.DisplayName
	}
	email := user.Email
	if req.Email != "" {
		email = req.Email
	}

	if err := h.db.UpdateUser(id, email, displayName, isActive); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}

// DeleteUser handles DELETE /api/users/:id.
func (h *UserHandler) DeleteUser(c *gin.Context) {
	callerID, _ := c.Get(middleware.CtxUserID)
	id := c.Param("id")
	if callerID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
		return
	}
	if err := h.db.DeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete user"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

// AssignRole handles POST /api/users/:id/roles.
func (h *UserHandler) AssignRole(c *gin.Context) {
	userID := c.Param("id")
	callerID, _ := c.Get(middleware.CtxUserID)
	var req struct {
		RoleID string `json:"role_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role_id is required"})
		return
	}
	callerStr, _ := callerID.(string)
	if err := h.db.AssignRole(userID, req.RoleID, callerStr); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign role"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "role assigned"})
}

// RevokeRole handles DELETE /api/users/:id/roles/:role_id.
func (h *UserHandler) RevokeRole(c *gin.Context) {
	userID := c.Param("id")
	roleID := c.Param("role_id")
	if err := h.db.RevokeRole(userID, roleID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke role"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "role revoked"})
}

// hashPassword is a convenience wrapper to avoid importing auth from handler.
func hashPassword(password string) (string, error) {
	return hashPasswordBcrypt(password)
}
