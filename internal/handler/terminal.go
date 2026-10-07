package handler

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"kestrel/internal/database"
	"kestrel/internal/middleware"
	"kestrel/internal/mcp"
	"kestrel/internal/processguard"
)

// TerminalHandler provides secure shell command execution for authorized operators.
type TerminalHandler struct {
	db       *database.DB
	registry *mcp.Registry
	logger   *zap.Logger
}

// NewTerminalHandler creates a TerminalHandler.
func NewTerminalHandler(db *database.DB, registry *mcp.Registry, logger *zap.Logger) *TerminalHandler {
	return &TerminalHandler{db: db, registry: registry, logger: logger}
}

type terminalExecReq struct {
	Command string `json:"command" binding:"required"`
	Cwd     string `json:"cwd"`
	Timeout int    `json:"timeout"` // seconds, default 30
}

type terminalExecResp struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// ExecCommand handles POST /api/terminal/exec.
func (h *TerminalHandler) ExecCommand(c *gin.Context) {
	var req terminalExecReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}

	cmdStr := strings.TrimSpace(req.Command)
	if cmdStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "empty command"})
		return
	}

	timeoutSec := req.Timeout
	if timeoutSec <= 0 || timeoutSec > 120 {
		timeoutSec = 30
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	start := time.Now()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", cmdStr)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/bash", "-c", cmdStr)
	}

	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}

	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// Process containment via processguard
	group, gErr := processguard.New(fmt.Sprintf("%x", time.Now().UnixNano()))
	var launch *processguard.Launch
	if gErr == nil && group != nil {
		launch, _ = group.Prepare(cmd)
	}
	defer func() {
		if group != nil {
			_ = group.Close(context.Background())
		}
	}()

	var runErr error
	if launch != nil {
		runErr = cmd.Start()
		if runErr == nil {
			_ = launch.Commit()
			runErr = cmd.Wait()
			launch.Dispose()
		}
	} else {
		runErr = cmd.Run()
	}
	durationMs := time.Since(start).Milliseconds()

	exitCode := 0
	errMsg := ""
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
			errMsg = runErr.Error()
		}
	}

	// Audit log execution
	actorID, _ := c.Get(middleware.CtxUserID)
	actorName, _ := c.Get(middleware.CtxUsername)
	aid, _ := actorID.(string)
	aname, _ := actorName.(string)

	_ = h.db.WriteAuditLog(database.AuditParams{
		ActorID:      aid,
		ActorName:    aname,
		Action:       "terminal.exec",
		Category:     "terminal",
		Result:       map[bool]string{true: "success", false: "failure"}[exitCode == 0],
		ResourceType: "command",
		ResourceID:   truncateString(cmdStr, 60),
		Message:      fmt.Sprintf("Executed terminal command: %s (code %d, %dms)", truncateString(cmdStr, 80), exitCode, durationMs),
		ClientIP:     c.ClientIP(),
		UserAgent:    c.Request.UserAgent(),
	})

	c.JSON(http.StatusOK, terminalExecResp{
		Stdout:     stdoutBuf.String(),
		Stderr:     stderrBuf.String(),
		ExitCode:   exitCode,
		DurationMs: durationMs,
		Error:      errMsg,
	})
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
