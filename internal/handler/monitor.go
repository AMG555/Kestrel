package handler

import (
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
	"kestrel/internal/database"
	"kestrel/internal/mcp"
	"kestrel/internal/storage"
)

var startTime = time.Now()

// MonitorHandler provides real-time health, resource usage, and runtime metrics.
type MonitorHandler struct {
	db       *database.DB
	registry *mcp.Registry
	dbPath   string
}

// NewMonitorHandler creates a MonitorHandler.
func NewMonitorHandler(db *database.DB, registry *mcp.Registry, dbPath string) *MonitorHandler {
	return &MonitorHandler{db: db, registry: registry, dbPath: dbPath}
}

// GetStatus handles GET /api/monitor/status.
func (h *MonitorHandler) GetStatus(c *gin.Context) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptimeSec := int64(time.Since(startTime).Seconds())

	// Database file size
	dbSizeBytes := int64(0)
	if fi, err := os.Stat(h.dbPath); err == nil {
		dbSizeBytes = fi.Size()
	}

	// Tool count
	registeredTools := 0
	if h.registry != nil {
		registeredTools = len(h.registry.ListTools())
	}

	// Database summary stats
	var userCount, sessionCount, projectCount, assetCount, vulnCount, toolExecCount, auditCount int
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM agent_sessions`).Scan(&sessionCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM projects`).Scan(&projectCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM assets`).Scan(&assetCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM vulnerabilities`).Scan(&vulnCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM tool_executions`).Scan(&toolExecCount)
	_ = h.db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&auditCount)
	dataSizeBytes, dataFileCount, _ := storage.CalculateDirUsage("./data")

	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"system": gin.H{
			"os":              runtime.GOOS,
			"arch":            runtime.GOARCH,
			"go_version":      runtime.Version(),
			"num_cpu":         runtime.NumCPU(),
			"goroutines":      runtime.NumGoroutine(),
			"uptime_secs":     uptimeSec,
			"memory_alloc_mb": float64(mem.Alloc) / 1024 / 1024,
			"memory_sys_mb":   float64(mem.Sys) / 1024 / 1024,
		},
		"database": gin.H{
			"size_bytes":      dbSizeBytes,
			"size_mb":         float64(dbSizeBytes) / 1024 / 1024,
			"users":           userCount,
			"sessions":        sessionCount,
			"projects":        projectCount,
			"assets":          assetCount,
			"vulnerabilities": vulnCount,
			"tool_executions": toolExecCount,
			"audit_logs":      auditCount,
		},
		"storage": gin.H{
			"data_dir_bytes":  dataSizeBytes,
			"data_dir_mb":     float64(dataSizeBytes) / 1024 / 1024,
			"data_file_count": dataFileCount,
		},
		"tools": gin.H{
			"registered_count": registeredTools,
		},
	})
}

