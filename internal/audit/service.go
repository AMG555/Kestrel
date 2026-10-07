package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
)

// Service persists platform audit logs with security sanitization and throttling.
type Service struct {
	db           *database.DB
	logger       *zap.Logger
	failThrottle *failureThrottle
	cooldownSec  int
	maxDetail    int
}

// NewService creates an audit service.
func NewService(db *database.DB, logger *zap.Logger) *Service {
	return &Service{
		db:           db,
		logger:       logger,
		failThrottle: newFailureThrottle(),
		cooldownSec:  5,
		maxDetail:    8192,
	}
}

// SetCooldown sets failure throttle cooldown in seconds.
func (s *Service) SetCooldown(sec int) {
	if sec > 0 {
		s.cooldownSec = sec
	}
}

// SetMaxDetailBytes sets the maximum serialized byte length for audit detail before truncation.
func (s *Service) SetMaxDetailBytes(n int) {
	if n > 0 {
		s.maxDetail = n
	}
}

// Record writes an audit log entry from a Gin request context.
func (s *Service) Record(c *gin.Context, e Entry) {
	if s == nil || s.db == nil {
		return
	}
	if strings.TrimSpace(e.Category) == "" || strings.TrimSpace(e.Action) == "" {
		return
	}
	if e.Result == ResultFailure && !s.allowFailure(c, e) {
		return
	}
	if strings.TrimSpace(e.Result) == "" {
		e.Result = ResultSuccess
	}
	if strings.TrimSpace(e.Level) == "" {
		if e.Result == ResultFailure || e.Result == ResultBlocked {
			e.Level = "warn"
		} else {
			e.Level = "info"
		}
	}

	actorID := e.ActorID
	actorName := e.ActorName
	if c != nil {
		if actorID == "" {
			if id, ok := c.Get(middleware.CtxUserID); ok {
				actorID, _ = id.(string)
			}
		}
		if actorName == "" {
			if name, ok := c.Get(middleware.CtxUsername); ok {
				actorName, _ = name.(string)
			}
		}
	}
	if actorName == "" {
		actorName = "system"
	}

	clientIP := e.ClientIP
	userAgent := e.UserAgent
	if c != nil {
		if clientIP == "" {
			clientIP = c.ClientIP()
		}
		if userAgent == "" {
			ua := c.GetHeader("User-Agent")
			if len(ua) > 512 {
				ua = ua[:512]
			}
			userAgent = ua
		}
	}

	sanitizedDetail := SanitizeDetail(e.Detail, s.maxDetail)

	err := s.db.WriteAuditLog(database.AuditParams{
		ActorID:      actorID,
		ActorName:    actorName,
		Action:       e.Action,
		Category:     e.Category,
		Result:       e.Result,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		ClientIP:     clientIP,
		UserAgent:    userAgent,
		Message:      e.Message,
		Detail:       sanitizedDetail,
	})
	if err != nil && s.logger != nil {
		s.logger.Warn("Failed to persist audit log",
			zap.String("category", e.Category),
			zap.String("action", e.Action),
			zap.Error(err),
		)
	}
}

// RecordSystem writes an audit record without HTTP context.
func (s *Service) RecordSystem(e Entry) {
	s.Record(nil, e)
}

// PurgeExpired deletes audit rows older than retentionDays.
func (s *Service) PurgeExpired(retentionDays int) (int64, error) {
	if s == nil || s.db == nil || retentionDays <= 0 {
		return 0, nil
	}
	deleted, err := s.db.PurgeOldAuditLogs(retentionDays)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to purge expired audit logs", zap.Error(err))
		}
		return 0, err
	}
	if deleted > 0 && s.logger != nil {
		s.logger.Info("Purged expired audit logs", zap.Int64("deleted", deleted))
	}
	return deleted, nil
}

// HintFromToken returns a 8-character hex SHA-256 prefix for session tokens.
func HintFromToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:4])
}

func (s *Service) allowFailure(c *gin.Context, e Entry) bool {
	if !IsAuthFailureThrottled(e.Category, e.Action) {
		return true
	}
	ip := e.ClientIP
	if c != nil {
		ip = c.ClientIP()
	}
	key := AuthFailureThrottleKey(e.Category, e.Action, ip)
	return s.failThrottle.allow(key, time.Duration(s.cooldownSec)*time.Second)
}
