//go:build windows

package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RunCommandWS: interactive PTY terminal depends on Unix PTY (see terminal_ws_unix.go); not supported on Windows.
func (h *TerminalHandler) RunCommandWS(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": "Interactive WebSocket terminal is not supported on Windows; use POST /terminal/run or /terminal/run/stream instead.",
	})
}
