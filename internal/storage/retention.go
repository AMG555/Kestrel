package storage

import (
	"errors"
	"time"

	"kestrel/internal/config"

	"go.uber.org/zap"
)

// minSweepInterval is the floor for background cleanup, preventing continuous disk traversal
// when interval_minutes is set to an extremely small value.
const minSweepInterval = 5 * time.Minute

// sweepCheckInterval is the wake-up granularity for the background loop.
// Sleeping for the full interval directly would mean a change from interval_minutes=60 to 5
// would not take effect until the current 60-minute sleep finishes. Short-granularity
// wake-ups combined with expiry checks limit the reaction delay to one granularity.
const sweepCheckInterval = time.Minute

// Service drives background automatic cleanup.
type Service struct {
	cleaner *Cleaner
	cfg     *config.Config
	logger  *zap.Logger
}

// NewService creates a background cleanup service.
func NewService(cleaner *Cleaner, cfg *config.Config, logger *zap.Logger) *Service {
	return &Service{cleaner: cleaner, cfg: cfg, logger: logger}
}

// AutoCleanEnabled reports whether background automatic cleanup is enabled (off by default).
func (s *Service) AutoCleanEnabled() bool {
	if s == nil || s.cfg == nil {
		return false
	}
	return s.cfg.Storage.AutoCleanEffective()
}

// Interval returns the cleanup interval; it is re-read on every cycle so config changes take effect without a restart.
func (s *Service) Interval() time.Duration {
	if s == nil || s.cfg == nil {
		return minSweepInterval
	}
	d := time.Duration(s.cfg.Storage.IntervalMinutesEffective()) * time.Minute
	if d < minSweepInterval {
		return minSweepInterval
	}
	return d
}

// PurgeExpired runs one round of automatic cleanup; it is a no-op when auto-cleanup is disabled.
func (s *Service) PurgeExpired() {
	if s == nil || s.cleaner == nil || !s.AutoCleanEnabled() {
		return
	}
	if _, err := s.cleaner.Clean(CleanRequest{Trigger: "schedule"}); err != nil {
		if s.logger != nil {
			// A concurrent conflict is not a fault: another round is already running, so just skip.
			if errors.Is(err, ErrCleanupInProgress) {
				s.logger.Debug("storage cleanup already in progress, skipping this round")
				return
			}
			s.logger.Warn("workspace auto-cleanup failed", zap.Error(err))
		}
	}
}

// StartRetentionLoop periodically cleans workspace garbage at the configured interval.
// It wakes on sweepCheckInterval granularity and only runs when due, so changes to
// interval_minutes take effect within at most one granularity.
func StartRetentionLoop(s *Service, logger *zap.Logger) {
	if s == nil || s.cleaner == nil {
		return
	}
	// Run one round immediately after startup to avoid waiting a full interval for the config to take effect;
	// placed in a goroutine so a full directory walk does not block the startup sequence.
	go func() {
		s.PurgeExpired()
		last := time.Now()
		ticker := time.NewTicker(sweepCheckInterval)
		defer ticker.Stop()
		for range ticker.C {
			if time.Since(last) < s.Interval() {
				continue
			}
			last = time.Now()
			s.PurgeExpired()
			if logger != nil {
				logger.Debug("storage retention tick completed")
			}
		}
	}()
}
