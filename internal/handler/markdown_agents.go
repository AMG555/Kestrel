package handler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"kestrel/internal/agents"
	"kestrel/internal/audit"
	"kestrel/internal/config"

	"github.com/gin-gonic/gin"
)

var markdownAgentFilenameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*\.md$`)

// MarkdownAgentsHandler manages sub-agent Markdown files under the agents directory (CRUD).
type MarkdownAgentsHandler struct {
	dir     string
	audit   *audit.Service
	writeMu sync.Mutex
}

// NewMarkdownAgentsHandler requires dir to be a resolved absolute path.
func NewMarkdownAgentsHandler(dir string) *MarkdownAgentsHandler {
	return &MarkdownAgentsHandler{dir: strings.TrimSpace(dir)}
}

// SetAudit wires platform audit logging.
func (h *MarkdownAgentsHandler) SetAudit(s *audit.Service) {
	h.audit = s
}

func (h *MarkdownAgentsHandler) safeJoin(filename string) (string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" || !markdownAgentFilenameRe.MatchString(filename) {
		return "", fmt.Errorf("invalid filename")
	}
	clean := filepath.Clean(filename)
	if clean != filename || strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid filename")
	}
	return filepath.Join(h.dir, clean), nil
}

// existingOtherOrchestrator returns the filename of another primary agent file in the directory if one exists in the same slot; no conflict when writingBasename is the file currently being written.
func existingOtherOrchestrator(dir, writingBasename string) (other string, err error) {
	load, err := agents.LoadMarkdownAgentsDir(dir)
	if err != nil {
		return "", err
	}
	wb := filepath.Base(strings.TrimSpace(writingBasename))
	switch agents.OrchestratorMarkdownKind(wb) {
	case "plan_execute":
		if load.OrchestratorPlanExecute != nil && !strings.EqualFold(load.OrchestratorPlanExecute.Filename, wb) {
			return load.OrchestratorPlanExecute.Filename, nil
		}
	case "supervisor":
		if load.OrchestratorSupervisor != nil && !strings.EqualFold(load.OrchestratorSupervisor.Filename, wb) {
			return load.OrchestratorSupervisor.Filename, nil
		}
	case "deep":
		if load.Orchestrator != nil && !strings.EqualFold(load.Orchestrator.Filename, wb) {
			return load.Orchestrator.Filename, nil
		}
	default:
		if load.Orchestrator != nil && !strings.EqualFold(load.Orchestrator.Filename, wb) {
			return load.Orchestrator.Filename, nil
		}
	}
	return "", nil
}

// ListMarkdownAgents GET /api/multi-agent/markdown-agents
func (h *MarkdownAgentsHandler) ListMarkdownAgents(c *gin.Context) {
	if h.dir == "" {
		c.JSON(http.StatusOK, gin.H{"agents": []any{}, "dir": "", "error": "agents directory not configured"})
		return
	}
	files, err := agents.LoadMarkdownAgentFiles(h.dir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]gin.H, 0, len(files))
	for _, fa := range files {
		sub := fa.Config
		out = append(out, gin.H{
			"filename":        fa.Filename,
			"id":              sub.ID,
			"name":            sub.Name,
			"description":     sub.Description,
			"is_orchestrator": fa.IsOrchestrator,
			"kind":            sub.Kind,
		})
	}
	c.JSON(http.StatusOK, gin.H{"agents": out, "dir": h.dir})
}

// GetMarkdownAgent GET /api/multi-agent/markdown-agents/:filename
func (h *MarkdownAgentsHandler) GetMarkdownAgent(c *gin.Context) {
	filename := c.Param("filename")
	path, err := h.safeJoin(filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	sub, err := agents.ParseMarkdownSubAgent(filename, string(b))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	isOrch := agents.IsOrchestratorLikeMarkdown(filename, sub.Kind)
	c.JSON(http.StatusOK, gin.H{
		"filename":        filename,
		"raw":             string(b),
		"id":              sub.ID,
		"name":            sub.Name,
		"description":     sub.Description,
		"tools":           sub.RoleTools,
		"instruction":     sub.Instruction,
		"bind_role":       sub.BindRole,
		"max_iterations":  sub.MaxIterations,
		"kind":            sub.Kind,
		"is_orchestrator": isOrch,
	})
}

type markdownAgentBody struct {
	Filename      string   `json:"filename"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Tools         []string `json:"tools"`
	Instruction   string   `json:"instruction"`
	BindRole      string   `json:"bind_role"`
	MaxIterations int      `json:"max_iterations"`
	Kind          string   `json:"kind"`
	Raw           string   `json:"raw"`
}

// CreateMarkdownAgent POST /api/multi-agent/markdown-agents
func (h *MarkdownAgentsHandler) CreateMarkdownAgent(c *gin.Context) {
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	if h.dir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agents directory not configured"})
		return
	}
	var body markdownAgentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filename := strings.TrimSpace(body.Filename)
	if filename == "" {
		if strings.EqualFold(strings.TrimSpace(body.Kind), "orchestrator") {
			filename = agents.OrchestratorMarkdownFilename
		} else {
			base := agents.SlugID(body.Name)
			if base == "" {
				base = "agent"
			}
			filename = base + ".md"
		}
	}
	path, err := h.safeJoin(filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := os.Stat(path); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "filealready exists"})
		return
	}
	sub := config.MultiAgentSubConfig{
		ID:            strings.TrimSpace(body.ID),
		Name:          strings.TrimSpace(body.Name),
		Description:   strings.TrimSpace(body.Description),
		Instruction:   strings.TrimSpace(body.Instruction),
		RoleTools:     body.Tools,
		BindRole:      strings.TrimSpace(body.BindRole),
		MaxIterations: body.MaxIterations,
		Kind:          strings.TrimSpace(body.Kind),
	}
	base := filepath.Base(path)
	if (strings.EqualFold(base, agents.OrchestratorMarkdownFilename) ||
		strings.EqualFold(base, agents.OrchestratorPlanExecuteMarkdownFilename) ||
		strings.EqualFold(base, agents.OrchestratorSupervisorMarkdownFilename)) && sub.Kind == "" {
		sub.Kind = "orchestrator"
	}
	if sub.ID == "" {
		sub.ID = agents.SlugID(sub.Name)
	}
	if sub.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	var out []byte
	if strings.TrimSpace(body.Raw) != "" {
		out = []byte(body.Raw)
	} else {
		out, err = agents.BuildMarkdownFile(sub)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := h.validateMarkdownWrite(filepath.Base(path), out); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if want := agents.WantsMarkdownOrchestrator(filepath.Base(path), body.Kind, string(out)); want {
		other, oerr := existingOtherOrchestrator(h.dir, filepath.Base(path))
		if oerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": oerr.Error()})
			return
		}
		if other != "" {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("a primary agent definition already exists: %s, please delete or remove its primary agent marker first", other)})
			return
		}
	}
	if err := os.MkdirAll(h.dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "filealready exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	_, writeErr := f.Write(out)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write agent file"})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "agent", "markdown_create", "create Markdown sub-agent", "markdown_agent", filepath.Base(path), nil)
	}
	c.JSON(http.StatusOK, gin.H{"filename": filepath.Base(path), "message": "created"})
}

// UpdateMarkdownAgent PUT /api/multi-agent/markdown-agents/:filename
func (h *MarkdownAgentsHandler) UpdateMarkdownAgent(c *gin.Context) {
	h.writeMu.Lock()
	defer h.writeMu.Unlock()
	filename := c.Param("filename")
	path, err := h.safeJoin(filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := os.Stat(path); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	var body markdownAgentBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sub := config.MultiAgentSubConfig{
		ID:            strings.TrimSpace(body.ID),
		Name:          strings.TrimSpace(body.Name),
		Description:   strings.TrimSpace(body.Description),
		Instruction:   strings.TrimSpace(body.Instruction),
		RoleTools:     body.Tools,
		BindRole:      strings.TrimSpace(body.BindRole),
		MaxIterations: body.MaxIterations,
		Kind:          strings.TrimSpace(body.Kind),
	}
	if (strings.EqualFold(filename, agents.OrchestratorMarkdownFilename) ||
		strings.EqualFold(filename, agents.OrchestratorPlanExecuteMarkdownFilename) ||
		strings.EqualFold(filename, agents.OrchestratorSupervisorMarkdownFilename)) && sub.Kind == "" {
		sub.Kind = "orchestrator"
	}
	if sub.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if sub.ID == "" {
		sub.ID = agents.SlugID(sub.Name)
	}
	var out []byte
	if strings.TrimSpace(body.Raw) != "" {
		out = []byte(body.Raw)
	} else {
		out, err = agents.BuildMarkdownFile(sub)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if err := h.validateMarkdownWrite(filename, out); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if want := agents.WantsMarkdownOrchestrator(filename, body.Kind, string(out)); want {
		other, oerr := existingOtherOrchestrator(h.dir, filename)
		if oerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": oerr.Error()})
			return
		}
		if other != "" {
			c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("a primary agent definition already exists: %s, please delete or remove its primary agent marker first", other)})
			return
		}
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "agent", "markdown_update", "update Markdown sub-agent", "markdown_agent", filename, nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "saved"})
}

// DeleteMarkdownAgent DELETE /api/multi-agent/markdown-agents/:filename
func (h *MarkdownAgentsHandler) DeleteMarkdownAgent(c *gin.Context) {
	filename := c.Param("filename")
	path, err := h.safeJoin(filename)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "agent", "markdown_delete", "delete Markdown sub-agent", "markdown_agent", filename, nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
