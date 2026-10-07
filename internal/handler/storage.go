package handler

import (
	"errors"
	"net/http"
	"strconv"

	"kestrel/internal/audit"
	"kestrel/internal/config"
	"kestrel/internal/storage"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// StorageHandler provides runtime storage usage stats and garbage cleanup API.
type StorageHandler struct {
	cleaner *storage.Cleaner
	cfg     *config.Config
	audit   *audit.Service
	logger  *zap.Logger
}

// NewStorageHandler creates the storage cleanup handler.
func NewStorageHandler(cleaner *storage.Cleaner, cfg *config.Config, logger *zap.Logger) *StorageHandler {
	return &StorageHandler{cleaner: cleaner, cfg: cfg, logger: logger}
}

// SetAudit wires platform audit logging.
func (h *StorageHandler) SetAudit(s *audit.Service) {
	if h != nil {
		h.audit = s
	}
}

// storageCleanupRequest is the request body for POST /api/storage/cleanup.
type storageCleanupRequest struct {
	// DryRun treated as true if omitted: only counts, does not delete. To actually delete, must explicitly pass false.
	DryRun *bool `json:"dry_run"`
	// When Confirm is false, execution is refused even if dry_run=false.
	// Disk deletion is irreversible; confirm must be an explicit API-level action, not just relying on a frontend dialog.
	Confirm    bool     `json:"confirm"`
	Categories []string `json:"categories"`
}

// Meta GET /api/storage/meta returns cleanup policy and per-category metadata.
func (h *StorageHandler) Meta(c *gin.Context) {
	st := h.effectiveConfig()
	items := make([]gin.H, 0, len(config.StorageCategoryOrder))
	for _, info := range storage.DescribeCategories() {
		items = append(items, gin.H{
			"key":               info.Key,
			"label":             info.Label,
			"hint":              info.Hint,
			"enabled":           st.CategoryEnabled(info.Key),
			"retention_days":    st.CategoryRetentionDays(info.Key),
			"default_retention": config.StorageCategoryDefaults[info.Key],
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"auto_clean":         st.AutoCleanEffective(),
		"interval_minutes":   st.IntervalMinutesEffective(),
		"orphan_grace_days":  st.OrphanGraceDaysEffective(),
		"active_grace_hours": st.ActiveGraceHoursEffective(),
		"categories":         items,
	})
}

// Status GET /api/storage/status returns filesystem capacity and per-category usage/recoverable amounts.
// ?refresh=1 forces re-traversal of directories; otherwise uses a short TTL cache.
func (h *StorageHandler) Status(c *gin.Context) {
	if h.cleaner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "storage cleanup not initialized"})
		return
	}
	refresh, _ := strconv.ParseBool(c.Query("refresh"))
	rep := h.cleaner.Inspect(refresh)
	c.JSON(http.StatusOK, gin.H{
		"filesystem":  rep.Filesystem,
		"categories":  rep.Categories,
		"totals":      rep.Totals,
		"scanned_at":  rep.StartedAt,
		"duration_ms": rep.DurationMS,
	})
}

// Cleanup POST /api/storage/cleanup executes cleanup (or preview).
func (h *StorageHandler) Cleanup(c *gin.Context) {
	if h.cleaner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "storage cleanup not initialized"})
		return
	}
	var req storageCleanupRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
			return
		}
	}
	dryRun := req.DryRun == nil || *req.DryRun
	if !dryRun && !req.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": "deletion is irreversible; to perform actual cleanup both dry_run=false and confirm=true must be passed"})
		return
	}

	rep, err := h.cleaner.Clean(storage.CleanRequest{
		DryRun:     dryRun,
		Categories: req.Categories,
		Trigger:    "manual",
	})
	switch {
	case errors.Is(err, storage.ErrCleanupInProgress):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	case errors.Is(err, storage.ErrUnknownCategory):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !dryRun {
		h.recordCleanup(c, rep)
	}
	c.JSON(http.StatusOK, gin.H{
		"dry_run":     rep.DryRun,
		"filesystem":  rep.Filesystem,
		"categories":  rep.Categories,
		"totals":      rep.Totals,
		"duration_ms": rep.DurationMS,
	})
}

func (h *StorageHandler) recordCleanup(c *gin.Context, rep *storage.Report) {
	if h.audit == nil {
		return
	}
	detail := map[string]interface{}{
		"removed_units": rep.Totals.RemovedUnits,
		"freed_bytes":   rep.Totals.FreedBytes,
		"errors":        rep.Totals.Errors,
	}
	if rep.Totals.Errors > 0 {
		h.audit.RecordFail(c, "storage", "cleanup", "runtime cleanup completed but some items failed", detail)
		return
	}
	h.audit.RecordOK(c, "storage", "cleanup", "cleaned up runtime garbage", "storage", "", detail)
}

func (h *StorageHandler) effectiveConfig() config.StorageConfig {
	if h.cfg == nil {
		return config.StorageConfig{}
	}
	return h.cfg.Storage
}
