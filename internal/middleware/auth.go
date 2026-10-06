package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"kestrel/internal/auth"
)

const (
	CtxUserID   = "user_id"
	CtxUsername = "username"
	CtxMustChangePwd = "must_change_password"
)

// Auth is a Gin middleware that validates the JWT from the Authorization header.
// It populates CtxUserID and CtxUsername in the Gin context.
func Auth(svc *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := auth.ExtractBearerToken(c.Request)
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		claims, err := svc.ValidateToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUsername, claims.Username)
		c.Set(CtxMustChangePwd, claims.MustChangePassword)
		c.Next()
	}
}

// RequirePasswordChange aborts with 403 if the user still has the forced-change flag set.
func RequirePasswordChange() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Exempt the change-password endpoint itself.
		if strings.HasSuffix(c.FullPath(), "/auth/change-password") {
			c.Next()
			return
		}
		if must, ok := c.Get(CtxMustChangePwd); ok {
			if mustBool, _ := must.(bool); mustBool {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
					"error": "password change required",
					"code":  "MUST_CHANGE_PASSWORD",
				})
				return
			}
		}
		c.Next()
	}
}

// RequirePermission aborts with 403 if the authenticated user does not hold the given permission.
func RequirePermission(permission string, checker func(userID, perm string) (bool, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, _ := c.Get(CtxUserID)
		uid, _ := userID.(string)
		if uid == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}

		ok, err := checker(uid, permission)
		if err != nil || !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}

// AuditContext enriches the Gin context with request metadata for audit logging.
func AuditContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("client_ip", c.ClientIP())
		c.Set("user_agent", c.Request.UserAgent())
		c.Next()
	}
}
