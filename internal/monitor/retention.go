package monitor

import (
	"context"
	"time"

	"go.uber.org/zap"
	"kestrel/internal/database"
)

const defaultPurgeInterval = 24 * time.Hour

// RetentionService periodically purges completed tool executions older than retentionDays.
type RetentionService struct {
	db     *database.DB
	logger *zap.Logger
}

// NewRetentionService creates a new tool execution retention service.
func NewRetentionService(db *database.DB, logger *zap.Logger) *RetentionService {
	return &RetentionService{
		db:     db,
		logger: logger,
	}
}

// PurgeExpired deletes tool execution records older than retentionDays.
func (s *RetentionService) PurgeExpired(retentionDays int) int64 {
	if s == nil || s.db == nil || retentionDays <= 0 {
		return 0
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	n, err := s.db.PurgeToolExecutionsBefore(cutoff)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to purge expired tool executions", zap.Error(err))
		}
		return 0
	}
	if n > 0 && s.logger != nil {
		s.logger.Info("Purged expired tool executions", zap.Int64("deleted", n), zap.Int("retention_days", retentionDays))
	}
	return n
}

// StartRetentionLoop starts the background purge worker.
func (s *RetentionService) StartRetentionLoop(ctx context.Context, retentionDays int, interval time.Duration) {
	if s == nil || retentionDays <= 0 {
		return
	}
	if interval <= 0 {
		interval = defaultPurgeInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// Initial purge on launch
		s.PurgeExpired(retentionDays)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.PurgeExpired(retentionDays)
			}
		}
	}()
}
