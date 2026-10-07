package handler

import (
	"strings"

	"kestrel/internal/config"
)

// effectiveProjectID prefers the project explicitly specified in the request/queue, falling back to config.project.default_project_id.
func effectiveProjectID(cfg *config.Config, explicit string) string {
	if pid := strings.TrimSpace(explicit); pid != "" {
		return pid
	}
	if cfg != nil {
		return strings.TrimSpace(cfg.Project.DefaultProjectID)
	}
	return ""
}
