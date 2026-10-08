package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kestrel/internal/agent"
	"kestrel/internal/audit"
	"kestrel/internal/authctx"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/mcp/builtin"
	"kestrel/internal/multiagent"
	"kestrel/internal/openai"
	"kestrel/internal/reasoning"
	"kestrel/internal/security"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// safeTruncateString safely truncate string, avoiding cuts in the middle of UTF-8 characters
func safeTruncateString(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}

	// convert string to rune slice to correctly count characters
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}

	// truncate to maximum length
	truncated := string(runes[:maxLen])

	// try to break at punctuation or space for a more natural truncation
	// search backwards from truncation point for a suitable break (no more than 20% of length)
	searchRange := maxLen / 5
	if searchRange > maxLen {
		searchRange = maxLen
	}
	breakChars := []rune(", .;:!?!?/\\-_")
	bestBreakPos := len(runes[:maxLen])

	for i := bestBreakPos - 1; i >= bestBreakPos-searchRange && i >= 0; i-- {
		for _, breakChar := range breakChars {
			if runes[i] == breakChar {
				bestBreakPos = i + 1 // break after punctuation
				goto found
			}
		}
	}

found:
	truncated = string(runes[:bestBreakPos])
	return truncated + "..."
}

// responsePlanAgg buffers main-assistant response_stream chunks for one "planning" process_detail row.
type responsePlanAgg struct {
	meta            map[string]interface{}
	b               strings.Builder
	detailID        string
	lastPersistAt   time.Time
	lastPersistSize int
}

// thinkingBuf aggregates thinking_stream_* / reasoning_chain_stream_* before flush to process_details.
type thinkingBuf struct {
	b         strings.Builder
	meta      map[string]interface{}
	persistAs string // "thinking" | "reasoning_chain"
}

func normalizeProcessDetailText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

// discardPlanningIfEchoesToolResult drops buffered planning text when it only repeats the
// upcoming tool_result body. Streaming models often echo tool stdout in chunk.Content; flushing
// that into "planning" before persisting tool_result duplicates the output after page refresh.
// sameResponseStreamMeta determine if it is the same primary channel stream (Eino ADK may send response_start multiple times for the same MessageStream).
func sameResponseStreamMeta(a, b map[string]interface{}) bool {
	if a == nil || b == nil {
		return false
	}
	agentA, _ := a["einoAgent"].(string)
	agentB, _ := b["einoAgent"].(string)
	agentA = strings.TrimSpace(agentA)
	agentB = strings.TrimSpace(agentB)
	if agentA == "" || !strings.EqualFold(agentA, agentB) {
		return false
	}
	orchA, _ := a["orchestration"].(string)
	orchB, _ := b["orchestration"].(string)
	if strings.TrimSpace(orchA) != strings.TrimSpace(orchB) {
		return false
	}
	iterA := responseStreamIterationFromMeta(a)
	iterB := responseStreamIterationFromMeta(b)
	if iterA != 0 && iterB != 0 && iterA != iterB {
		return false
	}
	streamA, _ := a["streamId"].(string)
	streamB, _ := b["streamId"].(string)
	streamA = strings.TrimSpace(streamA)
	streamB = strings.TrimSpace(streamB)
	if streamA != "" && streamB != "" && streamA != streamB {
		return false
	}
	return true
}

func responseStreamIterationFromMeta(m map[string]interface{}) int {
	if m == nil {
		return 0
	}
	switch v := m["iteration"].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func discardPlanningIfEchoesToolResult(respPlan *responsePlanAgg, toolData interface{}) string {
	if respPlan == nil {
		return ""
	}
	plan := normalizeProcessDetailText(respPlan.b.String())
	if plan == "" {
		return ""
	}
	dataMap, ok := toolData.(map[string]interface{})
	if !ok {
		return ""
	}
	res, ok := dataMap["result"].(string)
	if !ok {
		return ""
	}
	r := normalizeProcessDetailText(res)
	if r == "" {
		return ""
	}
	if plan == r || strings.HasSuffix(plan, r) {
		detailID := respPlan.detailID
		respPlan.meta = nil
		respPlan.b.Reset()
		respPlan.detailID = ""
		respPlan.lastPersistAt = time.Time{}
		respPlan.lastPersistSize = 0
		return detailID
	}
	return ""
}

// AgentHandler is the agent handler
type AgentHandler struct {
	agent            *agent.Agent
	db               *database.DB
	logger           *zap.Logger
	tasks            *AgentTaskManager
	taskEventBus     *TaskEventBus // mirror SSE events for re-subscribing to the same running task after refresh
	batchTaskManager *BatchTaskManager
	hitlManager      *HITLManager
	config           *config.Config // config reference, used to get role info
	knowledgeManager interface {    // knowledge base manager interface
		LogRetrieval(conversationID, messageID, query, riskType string, retrievedItems []string) error
	}
	agentsMarkdownDir string // multi-agent: Markdown sub-agent directory (absolute path, empty means do not merge from disk)
	batchCronParser   cron.Parser
	// hitlWhitelistSaver merges the session incremental whitelist into config.yaml when applying HITL from the sidebar (optional)
	hitlWhitelistSaver       HitlToolWhitelistSaver
	hitlStrategySaver        HitlAuditStrategySaver
	hitlDefaultReviewerSaver HitlDefaultReviewerSaver
	auditLLM                 *openai.Client
	audit                    *audit.Service
}

// SetAudit wires platform audit logging.
func (h *AgentHandler) SetAudit(s *audit.Service) {
	h.audit = s
}

// TaskManager returns the Agent task manager (used to terminate Eino executions, etc. from MCP monitor page).
func (h *AgentHandler) TaskManager() *AgentTaskManager {
	if h == nil {
		return nil
	}
	return h.tasks
}

// CancelRunningTaskForConversation stops any in-flight agent work for the conversation (idempotent).
func (h *AgentHandler) CancelRunningTaskForConversation(conversationID string) {
	if h == nil || conversationID == "" || h.tasks == nil {
		return
	}
	ok, err := h.tasks.CancelTask(conversationID, ErrTaskCancelled)
	if !ok {
		h.cancelRunningMCPToolsForConversation(conversationID)
		h.tasks.AbortActiveEinoExecute(conversationID, "")
	}
	if h.logger != nil {
		if err != nil {
			h.logger.Warn("failed to cancel conversation running task", zap.String("conversationId", conversationID), zap.Error(err))
		} else if ok {
			h.logger.Info("cancelled conversation running task", zap.String("conversationId", conversationID))
		}
	}

}

// ConversationTaskRuntimeState exposes the authoritative live state and start
// time used to scope persisted TaskCreate files to the current run. A task
// already entering cancellation must stop driving progress UI immediately.
func (h *AgentHandler) ConversationTaskRuntimeState(conversationID string) (bool, time.Time) {
	if h == nil || h.tasks == nil || strings.TrimSpace(conversationID) == "" {
		return false, time.Time{}
	}
	task := h.tasks.GetTaskSnapshot(strings.TrimSpace(conversationID))
	if task == nil || !strings.EqualFold(strings.TrimSpace(task.Status), "running") {
		return false, time.Time{}
	}
	return true, task.StartedAt
}

func (h *AgentHandler) cancelRunningMCPToolsForConversation(conversationID string) {
	if h == nil || h.agent == nil {
		return
	}
	n := h.agent.CancelRunningMCPToolsForConversation(conversationID, "conversation ended, automatically terminating still-running tools")
	if n > 0 && h.logger != nil {
		h.logger.Info("terminated still-running MCP tools for conversation", zap.String("conversationId", conversationID), zap.Int("count", n))
	}
}

// HitlToolWhitelistSaver merges/sets HITL whitelist tools to global config and persists to disk
type HitlToolWhitelistSaver interface {
	MergeHitlToolWhitelistIntoConfig(add []string) error
	SetHitlToolWhitelist(tools []string) error
}

// NewAgentHandler create a new Agent handler
func NewAgentHandler(agent *agent.Agent, db *database.DB, cfg *config.Config, logger *zap.Logger) *AgentHandler {
	batchTaskManager := NewBatchTaskManager(logger)
	batchTaskManager.SetDB(db)

	// load all batch task queues from database
	if err := batchTaskManager.LoadFromDB(); err != nil {
		logger.Warn("failed to load batch task queues from database", zap.Error(err))
	}

	bus := NewTaskEventBus()
	tm := NewAgentTaskManager()
	tm.SetTaskEventBus(bus)
	llmHTTP := &http.Client{Timeout: 2 * time.Minute}
	var llmCfg *config.OpenAIConfig
	if cfg != nil {
		llmCfg = &cfg.OpenAI
	}
	handler := &AgentHandler{
		agent:            agent,
		db:               db,
		logger:           logger,
		tasks:            tm,
		taskEventBus:     bus,
		batchTaskManager: batchTaskManager,
		config:           cfg,
		hitlManager:      NewHITLManager(db, logger),
		batchCronParser:  cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
		auditLLM:         openai.NewClient(llmCfg, llmHTTP, logger),
	}
	tm.SetToolCanceler(handler.cancelRunningMCPToolsForConversation)
	if err := handler.hitlManager.EnsureSchema(); err != nil {
		logger.Warn("failed to initialize HITL table", zap.Error(err))
	}
	go handler.batchQueueSchedulerLoop()
	return handler
}

// SetKnowledgeManager set knowledge base manager (for recording retrieval logs)
func (h *AgentHandler) SetKnowledgeManager(manager interface {
	LogRetrieval(conversationID, messageID, query, riskType string, retrievedItems []string) error
}) {
	h.knowledgeManager = manager
}

// SetAgentsMarkdownDir set agents/*.md sub-agent directory (absolute path); empty means use only sub_agents from config.yaml.
func (h *AgentHandler) SetAgentsMarkdownDir(absDir string) {
	h.agentsMarkdownDir = strings.TrimSpace(absDir)
}

// SetHitlToolWhitelistSaver set HITL whitelist persistence (paired with ConfigHandler to avoid circular references via interface)
func (h *AgentHandler) SetHitlToolWhitelistSaver(s HitlToolWhitelistSaver) {
	h.hitlWhitelistSaver = s
}

// HitlDefaultReviewerSaver persists the global default HITL config to config.yaml.
type HitlDefaultReviewerSaver interface {
	UpdateHitlDefaultConfig(mode, reviewer string, timeoutSeconds int) error
	UpdateHitlDefaultReviewer(reviewer string) error
}

// SetHitlDefaultReviewerSaver set HITL default config persistence.
func (h *AgentHandler) SetHitlDefaultReviewerSaver(s HitlDefaultReviewerSaver) {
	h.hitlDefaultReviewerSaver = s
}

func (h *AgentHandler) hitlEffectiveDefaultReviewer() string {
	if h != nil && h.config != nil {
		return normalizeHitlReviewer(h.config.Hitl.EffectiveDefaultReviewer())
	}
	return "human"
}

func (h *AgentHandler) hitlEffectiveDefaultMode() string {
	if h != nil && h.config != nil {
		return normalizeHitlDefaultMode(h.config.Hitl.EffectiveDefaultMode())
	}
	return "off"
}

func (h *AgentHandler) hitlEffectiveDefaultTimeoutSeconds() int {
	if h != nil && h.config != nil {
		timeout := h.config.Hitl.EffectiveDefaultTimeoutSeconds()
		if timeout < 0 {
			return 0
		}
		return timeout
	}
	return 300
}

func (h *AgentHandler) hitlEffectiveDefaultRequest() *HITLRequest {
	mode := h.hitlEffectiveDefaultMode()
	return &HITLRequest{
		Enabled:        mode != "off",
		Mode:           mode,
		Reviewer:       h.hitlEffectiveDefaultReviewer(),
		SensitiveTools: []string{},
		TimeoutSeconds: h.hitlEffectiveDefaultTimeoutSeconds(),
	}
}

// HITLNeedsToolApproval for C2 dangerous task gating: consistent with session-side HITL and whitelist approval logic.
func (h *AgentHandler) HITLNeedsToolApproval(conversationID, toolName string) bool {
	if h == nil || h.hitlManager == nil {
		return false
	}
	return h.hitlManager.NeedsToolApproval(conversationID, toolName)
}

// ChatAttachment is a chat attachment (file uploaded by user)
type ChatAttachment struct {
	FileName   string `json:"fileName"`          // display filename
	Content    string `json:"content,omitempty"` // text or base64; can be left empty if already pre-uploaded to server
	MimeType   string `json:"mimeType,omitempty"`
	ServerPath string `json:"serverPath,omitempty"` // absolute path already saved under chat_uploads (returned by POST /api/chat-uploads)
}

// ChatReasoningRequest is the 'model reasoning' intent from the conversation page (consumed by Eino single/multi-agent path).
type ChatReasoningRequest struct {
	// Mode: default (follows system) | off | on | auto
	Mode string `json:"mode,omitempty"`
	// Effort: low | medium | high | max | xhigh (passed through as-is; different gateways use different names for the highest tier). Null means unspecified.
	Effort string `json:"effort,omitempty"`
}

// ChatFinalizationRequest is a caller-provided delivery policy. The server does
// not infer execution intent from natural-language user text.
type ChatFinalizationRequest struct {
	RequireExecutionEvidence *bool `json:"requireExecutionEvidence,omitempty"`
}

// ChatRequest is the chat request
type ChatRequest struct {
	Message              string                  `json:"message" binding:"required"`
	ConversationID       string                  `json:"conversationId,omitempty"`
	ProjectID            string                  `json:"projectId,omitempty"` // project to bind for new conversation (optional; config.project.default_project_id used when not specified)
	Role                 string                  `json:"role,omitempty"`      // role name
	Attachments          []ChatAttachment        `json:"attachments,omitempty"`
	WebShellConnectionID string                  `json:"webshellConnectionId,omitempty"` // WebShell management - AI assistant: currently selected connection ID, only uses webshell_* tools
	AIChannelID          string                  `json:"aiChannelId,omitempty"`          // session-level AI channel; uses ai.default_channel when empty
	Hitl                 *HITLRequest            `json:"hitl,omitempty"`
	Reasoning            *ChatReasoningRequest   `json:"reasoning,omitempty"`
	Finalization         ChatFinalizationRequest `json:"finalization,omitempty"`
	// Orchestration only for /api/multi-agent, /api/multi-agent/stream: deep | plan_execute | supervisor; empty equals deep. Server defaults to deep for robots/batch etc. with no request body. /api/eino-agent* does not use this field.
	Orchestration string `json:"orchestration,omitempty"`
}

func (h *AgentHandler) configForAIChannel(channelID string) (*config.Config, string, error) {
	if h == nil || h.config == nil {
		return nil, "", fmt.Errorf("server configuration not loaded")
	}
	oa, resolvedID, ok := h.config.ResolveAIChannel(channelID)
	if !ok {
		return nil, resolvedID, fmt.Errorf("AI channel not found: %s", resolvedID)
	}
	cfgCopy := *h.config
	cfgCopy.OpenAI = oa
	return &cfgCopy, resolvedID, nil
}

func chatReasoningToClientIntent(r *ChatReasoningRequest) *reasoning.ClientIntent {
	if r == nil {
		return nil
	}
	return &reasoning.ClientIntent{Mode: r.Mode, Effort: r.Effort}
}

type HITLRequest struct {
	Enabled        bool     `json:"enabled"`
	Mode           string   `json:"mode,omitempty"`
	Reviewer       string   `json:"reviewer,omitempty"` // human | audit_agent
	SensitiveTools []string `json:"sensitiveTools,omitempty"`
	TimeoutSeconds int      `json:"timeoutSeconds,omitempty"`
}

const (
	maxAttachments     = 10
	chatUploadsDirName = "chat_uploads" // root directory for saving conversation attachments (relative to current working directory)
)

// validateChatAttachmentServerPath verify absolute path is under working directory chat_uploads and is a regular file (prevent path traversal)
func validateChatAttachmentServerPath(abs string) (string, error) {
	p := strings.TrimSpace(abs)
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}
	root := filepath.Join(cwd, chatUploadsDirName)
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	pathAbs, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return "", err
	}
	sep := string(filepath.Separator)
	if pathAbs != rootAbs && !strings.HasPrefix(pathAbs, rootAbs+sep) {
		return "", fmt.Errorf("path outside chat_uploads")
	}
	st, err := os.Stat(pathAbs)
	if err != nil {
		return "", err
	}
	if st.IsDir() {
		return "", fmt.Errorf("not a regular file")
	}
	return pathAbs, nil
}

// avoidChatUploadDestCollision generates a new filename with timestamp+random suffix if the path already exists (consistent with upload interface naming style)
func avoidChatUploadDestCollision(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	nameNoExt := strings.TrimSuffix(base, ext)
	suffix := fmt.Sprintf("_%s_%s", time.Now().Format("150405"), shortRand(6))
	var unique string
	if ext != "" {
		unique = nameNoExt + suffix + ext
	} else {
		unique = base + suffix
	}
	return filepath.Join(dir, unique)
}

// relocateManualOrNewUploadToConversation when there is no conversation ID the frontend uploads to …/date/_manual; after the first message creates the conversation, move the file to …/date/{conversationId}/ for per-conversation isolation.
func relocateManualOrNewUploadToConversation(absPath, conversationID string, logger *zap.Logger) (string, error) {
	conv := strings.TrimSpace(conversationID)
	if conv == "" {
		return absPath, nil
	}
	convSan := strings.ReplaceAll(conv, string(filepath.Separator), "_")
	if convSan == "" || convSan == "_manual" || convSan == "_new" {
		return absPath, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return absPath, err
	}
	rootAbs, err := filepath.Abs(filepath.Join(cwd, chatUploadsDirName))
	if err != nil {
		return absPath, err
	}
	rel, err := filepath.Rel(rootAbs, absPath)
	if err != nil {
		return absPath, nil
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	var segs []string
	for _, p := range strings.Split(rel, "/") {
		if p != "" && p != "." {
			segs = append(segs, p)
		}
	}
	// only handle flat structure: date/_manual|_new/filename
	if len(segs) != 3 {
		return absPath, nil
	}
	datePart, placeFolder, baseName := segs[0], segs[1], segs[2]
	if placeFolder != "_manual" && placeFolder != "_new" {
		return absPath, nil
	}
	targetDir := filepath.Join(rootAbs, datePart, convSan)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create conversation attachment directory: %w", err)
	}
	dest := filepath.Join(targetDir, baseName)
	dest = avoidChatUploadDestCollision(dest)
	if err := os.Rename(absPath, dest); err != nil {
		return "", fmt.Errorf("failed to move attachment into conversation directory: %w", err)
	}
	out, _ := filepath.Abs(dest)
	if logger != nil {
		logger.Info("conversation attachment moved from placeholder directory to conversation directory",
			zap.String("from", absPath),
			zap.String("to", out),
			zap.String("conversationId", conv))
	}
	return out, nil
}

// saveAttachmentsToDateAndConversationDir process attachments: if serverPath is provided, only validate the existing file; otherwise write content to chat_uploads/YYYY-MM-DD/{conversationID}/.
// use "_new" as directory name when conversationID is empty (new conversation has no ID yet)
func saveAttachmentsToDateAndConversationDir(attachments []ChatAttachment, conversationID string, logger *zap.Logger) (savedPaths []string, err error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	dateDir := filepath.Join(cwd, chatUploadsDirName, time.Now().Format("2006-01-02"))
	convDirName := strings.TrimSpace(conversationID)
	if convDirName == "" {
		convDirName = "_new"
	} else {
		convDirName = strings.ReplaceAll(convDirName, string(filepath.Separator), "_")
	}
	targetDir := filepath.Join(dateDir, convDirName)
	if err = os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("create upload directory failed: %w", err)
	}
	savedPaths = make([]string, 0, len(attachments))
	for i, a := range attachments {
		if sp := strings.TrimSpace(a.ServerPath); sp != "" {
			valid, verr := validateChatAttachmentServerPath(sp)
			if verr != nil {
				return nil, fmt.Errorf("attachment %s: %w", a.FileName, verr)
			}
			finalPath, rerr := relocateManualOrNewUploadToConversation(valid, conversationID, logger)
			if rerr != nil {
				return nil, fmt.Errorf("attachment %s: %w", a.FileName, rerr)
			}
			savedPaths = append(savedPaths, finalPath)
			if logger != nil {
				logger.Debug("conversation attachment using already-uploaded path", zap.Int("index", i+1), zap.String("fileName", a.FileName), zap.String("path", finalPath))
			}
			continue
		}
		if strings.TrimSpace(a.Content) == "" {
			return nil, fmt.Errorf("attachment %s is missing content or serverPath was not provided", a.FileName)
		}
		raw, decErr := attachmentContentToBytes(a)
		if decErr != nil {
			return nil, fmt.Errorf("failed to decode attachment %s: %w", a.FileName, decErr)
		}
		baseName := filepath.Base(a.FileName)
		if baseName == "" || baseName == "." {
			baseName = "file"
		}
		baseName = strings.ReplaceAll(baseName, string(filepath.Separator), "_")
		ext := filepath.Ext(baseName)
		nameNoExt := strings.TrimSuffix(baseName, ext)
		suffix := fmt.Sprintf("_%s_%s", time.Now().Format("150405"), shortRand(6))
		var unique string
		if ext != "" {
			unique = nameNoExt + suffix + ext
		} else {
			unique = baseName + suffix
		}
		fullPath := filepath.Join(targetDir, unique)
		if err = os.WriteFile(fullPath, raw, 0644); err != nil {
			return nil, fmt.Errorf("failed to write file %s: %w", a.FileName, err)
		}
		absPath, _ := filepath.Abs(fullPath)
		savedPaths = append(savedPaths, absPath)
		if logger != nil {
			logger.Debug("conversation attachment saved", zap.Int("index", i+1), zap.String("fileName", a.FileName), zap.String("path", absPath))
		}
	}
	return savedPaths, nil
}

func shortRand(n int) string {
	const letters = "0123456789abcdef"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = letters[int(b[i])%len(letters)]
	}
	return string(b)
}

func attachmentContentToBytes(a ChatAttachment) ([]byte, error) {
	content := a.Content
	if decoded, err := base64.StdEncoding.DecodeString(content); err == nil && len(decoded) > 0 {
		return decoded, nil
	}
	return []byte(content), nil
}

// userMessageContentForStorage return user message content to store in database: when there are attachments, append attachment names (and paths) after the message body, so they remain visible after refresh and the model can retrieve paths from history when continuing the conversation
func userMessageContentForStorage(message string, attachments []ChatAttachment, savedPaths []string) string {
	if len(attachments) == 0 {
		return message
	}
	var b strings.Builder
	b.WriteString(message)
	for i, a := range attachments {
		b.WriteString("\n📎 ")
		b.WriteString(a.FileName)
		if i < len(savedPaths) && savedPaths[i] != "" {
			b.WriteString(": ")
			b.WriteString(savedPaths[i])
		}
	}
	return b.String()
}

// appendAttachmentsToMessage only appends the attachment save path to the end of the user message; no longer inlines attachment content to avoid context length issues
func appendAttachmentsToMessage(msg string, attachments []ChatAttachment, savedPaths []string) string {
	if len(attachments) == 0 {
		return msg
	}
	var b strings.Builder
	b.WriteString(msg)
	b.WriteString("\n\n[Files uploaded by user]\n")
	for i, a := range attachments {
		if i < len(savedPaths) && savedPaths[i] != "" {
			b.WriteString(fmt.Sprintf("- %s: %s\n", a.FileName, savedPaths[i]))
		} else {
			b.WriteString(fmt.Sprintf("- %s: (path unknown, may have failed to save)\n", a.FileName))
		}
	}
	return b.String()
}

// appendAssistantMessageNotice appends a notice to the end of an assistant message, avoiding overwriting already generated content.
// if message is empty, write the notice directly; if it already contains the same notice, leave it unchanged.
func (h *AgentHandler) appendAssistantMessageNotice(messageID, notice string) error {
	trimmedNotice := strings.TrimSpace(notice)
	if strings.TrimSpace(messageID) == "" || trimmedNotice == "" {
		return nil
	}
	_, err := h.db.Exec(
		`UPDATE messages
		 SET content = CASE
			WHEN content IS NULL OR TRIM(content) = '' THEN ?
			WHEN INSTR(content, ?) > 0 THEN content
			ELSE content || '\n\n' || ?
		 END,
		     updated_at = ?
		 WHERE id = ?`,
		trimmedNotice,
		trimmedNotice,
		trimmedNotice,
		time.Now(),
		messageID,
	)
	return err
}

// mergeAssistantMessagePartialOnCancel merges the partial reply generated before cancellation into the message:
// - if content is empty or placeholder only (processing...), directly replace with partial;
// - if body already exists, only append if it does not yet contain partial, to avoid loss and duplication.
func (h *AgentHandler) mergeAssistantMessagePartialOnCancel(messageID, partial string) error {
	trimmedPartial := strings.TrimSpace(partial)
	if strings.TrimSpace(messageID) == "" || trimmedPartial == "" {
		return nil
	}
	_, err := h.db.Exec(
		`UPDATE messages
		 SET content = CASE
			WHEN content IS NULL OR TRIM(content) = '' OR TRIM(content) = 'processing...' THEN ?
			WHEN INSTR(content, ?) > 0 THEN content
			ELSE content || '\n\n' || ?
		 END,
		     updated_at = ?
		 WHERE id = ?`,
		trimmedPartial,
		trimmedPartial,
		trimmedPartial,
		time.Now(),
		messageID,
	)
	return err
}

// ChatResponse is the chat response
type ChatResponse struct {
	Response                         string    `json:"response"`
	MCPExecutionIDs                  []string  `json:"mcpExecutionIds,omitempty"` // list of MCP call IDs executed in this conversation
	ConversationID                   string    `json:"conversationId"`            // conversation ID
	Time                             time.Time `json:"time"`
	Finalizable                      bool      `json:"finalizable"`
	Finalized                        bool      `json:"finalized"`
	Status                           string    `json:"status,omitempty"`
	CompletionReason                 string    `json:"completionReason,omitempty"`
	EvidenceVerified                 bool      `json:"evidenceVerified"`
	EvidenceRefs                     []string  `json:"evidenceRefs,omitempty"`
	PendingExecutionIDs              []string  `json:"pendingExecutionIds,omitempty"`
	MissingChecks                    []string  `json:"missingChecks,omitempty"`
	AutoCancelledPendingExecutionIDs []string  `json:"autoCancelledPendingExecutionIds,omitempty"`
}

func (h *AgentHandler) finalizeRobotAgentError(ctx context.Context, assistantMessageID, conversationID string, resultMA *multiagent.RunResult, errMA error) (string, string, error) {
	if shouldPersistEinoAgentTraceAfterRunError(ctx) {
		h.persistEinoAgentTraceForResume(conversationID, resultMA)
	}
	errMsg := "execution failed: " + multiagent.EinoClientRunErrorMessage(errMA)
	if assistantMessageID != "" {
		_, _ = h.db.Exec("UPDATE messages SET content = ?, updated_at = ? WHERE id = ?", errMsg, time.Now(), assistantMessageID)
		_ = h.db.AddProcessDetail(assistantMessageID, conversationID, "error", errMsg, nil)
	}
	return "", conversationID, errMA
}

func (h *AgentHandler) finalizeRobotAgentSuccess(taskCtx context.Context, assistantMessageID, conversationID string, resultMA *multiagent.RunResult) (string, string, error) {
	reasoningContent := multiagent.AggregatedReasoningFromTraceJSON(resultMA.LastAgentTraceInput)
	decision := h.decideAgentRunForDeliveryWithPolicy(conversationID, assistantMessageID, "robot", resultMA, resultMA.MCPExecutionIDs, false)
	if cancelled := h.cleanupPendingToolExecutionsAfterIteration(taskCtx, conversationID, decision, nil); len(cancelled) > 0 {
		decision = h.decideAgentRunForDeliveryWithPolicy(conversationID, assistantMessageID, "robot", resultMA, resultMA.MCPExecutionIDs, false)
	}
	h.persistFinalizationDecision(conversationID, assistantMessageID, "robot", resultMA.MCPExecutionIDs, reasoningContent, decision)
	responseText := decision.FinalText
	if !decision.Finalizable {
		responseText = finalizationBlockedMessage(decision)
	}
	if assistantMessageID == "" {
		if _, err := h.db.AddMessage(conversationID, "assistant", responseText, resultMA.MCPExecutionIDs); err != nil {
			h.logger.Warn("robot: failed to save assistant message", zap.Error(err))
		}
	}
	if resultMA.LastAgentTraceInput != "" || resultMA.LastAgentTraceOutput != "" {
		_ = h.db.SaveAgentTrace(conversationID, resultMA.LastAgentTraceInput, resultMA.LastAgentTraceOutput)
	}
	return responseText, conversationID, nil
}

func (h *AgentHandler) runRobotEinoSingleWithRetry(
	taskCtx context.Context,
	conversationID, finalMessage string,
	history []agent.ChatMessage,
	roleTools []string,
	progressCallback agent.ProgressCallback,
	assistantMessageID string,
	taskStatus *string,
) (string, string, error) {
	resultMA, errMA := multiagent.RunEinoSingleChatModelAgent(
		taskCtx, h.config, &h.config.MultiAgent, h.agent, h.db, h.logger,
		conversationID, h.conversationProjectID(conversationID), finalMessage, history, roleTools, progressCallback, nil, h.agentSessionContextBlock(conversationID),
	)
	if errMA != nil {
		*taskStatus = "failed"
		return h.finalizeRobotAgentError(taskCtx, assistantMessageID, conversationID, resultMA, errMA)
	}
	return h.finalizeRobotAgentSuccess(taskCtx, assistantMessageID, conversationID, resultMA)
}

func (h *AgentHandler) runRobotMultiAgentWithRetry(
	taskCtx context.Context,
	conversationID, finalMessage, orchestration string,
	history []agent.ChatMessage,
	roleTools []string,
	progressCallback agent.ProgressCallback,
	assistantMessageID string,
	taskStatus *string,
) (string, string, error) {
	resultMA, errMA := multiagent.RunDeepAgent(
		taskCtx, h.config, &h.config.MultiAgent, h.agent, h.db, h.logger,
		conversationID, h.conversationProjectID(conversationID), finalMessage, history, roleTools, progressCallback,
		h.agentsMarkdownDir, orchestration, nil, h.agentSessionContextBlock(conversationID),
	)
	if errMA != nil {
		*taskStatus = "failed"
		return h.finalizeRobotAgentError(taskCtx, assistantMessageID, conversationID, resultMA, errMA)
	}
	return h.finalizeRobotAgentSuccess(taskCtx, assistantMessageID, conversationID, resultMA)
}

// ProcessMessageForRobot is called by robots (WeCom/DingTalk/Feishu): Eino single/multi-agent execution path (includes progressCallback and process details), does not send SSE, returns full reply at the end
func (h *AgentHandler) ProcessMessageForRobot(ctx context.Context, platform string, principal authctx.Principal, conversationID, message, role, agentMode string) (response string, convID string, err error) {
	ownerUserID := strings.TrimSpace(principal.UserID)
	if ownerUserID == "" {
		return "", "", fmt.Errorf("authenticated robot principal is required")
	}
	if !principal.HasPermission("agent:execute") || !principal.HasPermission("chat:read") || !principal.HasPermission("chat:write") {
		return "", "", fmt.Errorf("robot account lacks agent:execute, chat:read, or chat:write permissions")
	}
	ctx = authctx.WithPrincipal(ctx, principal)
	if conversationID == "" {
		title := safeTruncateString(message, 50)
		src := "robot"
		if strings.TrimSpace(platform) != "" {
			src = "robot:" + strings.TrimSpace(platform)
		}
		meta := audit.ConversationCreateMeta(src)
		meta.ProjectID = effectiveProjectID(h.config, "")
		if meta.ProjectID != "" && (!principal.HasPermission("project:read") || !h.db.UserCanAccessResource(ownerUserID, principal.ScopeFor("project:read"), "project", meta.ProjectID)) {
			meta.ProjectID = ""
		}
		conv, createErr := h.db.CreateConversation(title, meta)
		if createErr != nil {
			return "", "", fmt.Errorf("failed to create conversation: %w", createErr)
		}
		conversationID = conv.ID
		_ = h.db.SetResourceOwner("conversation", conversationID, ownerUserID)
	} else {
		if _, getErr := h.db.GetConversation(conversationID); getErr != nil || !h.db.UserCanAccessResource(ownerUserID, principal.ScopeFor("chat:write"), "conversation", conversationID) {
			return "", "", fmt.Errorf("conversation not found")
		}
	}

	agentHistoryMessages, err := h.loadHistoryFromAgentTrace(conversationID)
	if err != nil {
		historyMessages, getErr := h.db.GetMessages(conversationID)
		if getErr != nil {
			agentHistoryMessages = []agent.ChatMessage{}
		} else {
			agentHistoryMessages = make([]agent.ChatMessage, 0, len(historyMessages))
			for _, msg := range historyMessages {
				agentHistoryMessages = append(agentHistoryMessages, agent.ChatMessage{Role: msg.Role, Content: msg.Content})
			}
		}
	}

	finalMessage := message
	var roleTools []string
	if role != "" && role != "default" && h.config.Roles != nil {
		if r, exists := h.config.Roles[role]; exists && r.Enabled {
			if r.UserPrompt != "" {
				finalMessage = r.UserPrompt + "\n\n" + message
			}
			roleTools = r.Tools
		}
	}

	if _, err = h.db.AddMessage(conversationID, "user", message, nil); err != nil {
		return "", "", fmt.Errorf("failed to save user message: %w", err)
	}

	// consistent with Eino streaming conversation: first create assistant message placeholder, use progressCallback to write process details (no SSE)
	assistantMsg, err := h.db.AddMessage(conversationID, "assistant", "processing...", nil)
	if err != nil {
		h.logger.Warn("robot: failed to create assistant message placeholder", zap.Error(err))
	}
	var assistantMessageID string
	if assistantMsg != nil {
		assistantMessageID = assistantMsg.ID
	}

	// register running task and mirror progress events to taskEventBus for Web task-events stream supplement.
	taskCtx, cancelWithCause := context.WithCancelCause(ctx)
	defer cancelWithCause(nil)
	taskStatus := "completed"
	var taskRunID string
	defer func() {
		if taskRunID == "" {
			return
		}
		if cleanupErr := h.tasks.FinishTaskRun(conversationID, taskRunID, taskStatus); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()
	if startedTask, err := h.tasks.StartTask(conversationID, message, cancelWithCause); err != nil {
		if errors.Is(err, ErrTaskAlreadyRunning) {
			return "", conversationID, fmt.Errorf("a task is already running in this conversation, please try again later")
		}
		return "", conversationID, fmt.Errorf("failed to start task: %w", err)
	} else {
		taskRunID = startedTask.RunID
	}
	taskCtx = h.tasks.BindProcessScope(taskCtx, conversationID, taskRunID)
	progressCallback := h.createProgressCallback(taskCtx, cancelWithCause, conversationID, assistantMessageID, nil)

	robotMode := config.NormalizeAgentMode(agentMode)
	if err := h.db.SetConversationAgentMode(conversationID, robotMode); err != nil {
		h.logger.Warn("robot: failed to update conversation mode", zap.String("conversationId", conversationID), zap.String("agentMode", robotMode), zap.Error(err))
	}
	switch robotMode {
	case "eino_single":
		return h.runRobotEinoSingleWithRetry(taskCtx, conversationID, finalMessage, agentHistoryMessages, roleTools, progressCallback, assistantMessageID, &taskStatus)
	case "deep", "plan_execute", "supervisor":
		if h.config == nil || !h.config.MultiAgent.Enabled {
			taskStatus = "failed"
			return "", conversationID, fmt.Errorf("robot conversation mode %s requires Eino multi-agent to be enabled", robotMode)
		}
		return h.runRobotMultiAgentWithRetry(taskCtx, conversationID, finalMessage, robotMode, agentHistoryMessages, roleTools, progressCallback, assistantMessageID, &taskStatus)
	}

	taskStatus = "failed"
	return "", conversationID, fmt.Errorf("unsupported robot agent mode: %s", robotMode)
}

// StreamEvent is a streaming event
type StreamEvent struct {
	Type    string      `json:"type"`    // conversation, progress, tool_call, tool_result, response, error, cancelled, done
	Message string      `json:"message"` // Display message
	Data    interface{} `json:"data,omitempty"`
}

// publishProgressToTaskEventBus mirrors progress events to the taskEventBus (for Web task-events subscription when robot / no HTTP SSE client).
func (h *AgentHandler) publishProgressToTaskEventBus(conversationID, eventType, message string, data interface{}) {
	if h == nil || h.taskEventBus == nil || strings.TrimSpace(conversationID) == "" {
		return
	}
	event := StreamEvent{Type: eventType, Message: message, Data: data}
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return
	}
	sseLine := make([]byte, 0, len(eventJSON)+8)
	sseLine = append(sseLine, []byte("data: ")...)
	sseLine = append(sseLine, eventJSON...)
	sseLine = append(sseLine, '\n', '\n')
	h.taskEventBus.Publish(conversationID, sseLine)
}

func isInternalEinoDiagnosticProgress(eventType, message string, data interface{}) bool {
	switch eventType {
	case "model_output_rejected":
		return true
	case "progress":
		msg := strings.TrimSpace(message)
		if msg == "Eino TurnLoop persistent multi-turn runtime has taken over this session." ||
			msg == "Eino TurnLoop has switched to the next round after user supplement at a safe point." ||
			msg == "User supplement has been pushed to Eino TurnLoop, waiting for safe-point switch..." {
			return true
		}
		m, ok := data.(map[string]interface{})
		if !ok {
			return false
		}
		switch strings.TrimSpace(fmt.Sprint(m["kind"])) {
		case "turn_loop_takeover", "turn_loop_preempted":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

// enrichProgressEventData fills in conversationId and messageId for SSE / taskEventBus events to enable frontend lazy loading of process details.
func enrichProgressEventData(data interface{}, conversationID, assistantMessageID string) interface{} {
	if strings.TrimSpace(conversationID) == "" && strings.TrimSpace(assistantMessageID) == "" {
		return data
	}
	var m map[string]interface{}
	switch v := data.(type) {
	case map[string]interface{}:
		m = make(map[string]interface{}, len(v)+2)
		for k, val := range v {
			m[k] = val
		}
	case nil:
		m = make(map[string]interface{}, 2)
	default:
		m = map[string]interface{}{"payload": data}
	}
	if id := strings.TrimSpace(assistantMessageID); id != "" {
		if existing, ok := m["messageId"]; !ok || strings.TrimSpace(fmt.Sprint(existing)) == "" {
			m["messageId"] = id
		}
	}
	if id := strings.TrimSpace(conversationID); id != "" {
		if existing, ok := m["conversationId"]; !ok || strings.TrimSpace(fmt.Sprint(existing)) == "" {
			m["conversationId"] = id
		}
	}
	return m
}

// createProgressCallback creates a progress callback function for saving process details
// sendEventFunc: optional streaming event send function; if nil, no streaming events are sent
func (h *AgentHandler) createProgressCallback(runCtx context.Context, cancelRun context.CancelCauseFunc, conversationID, assistantMessageID string, sendEventFunc func(eventType, message string, data interface{})) agent.ProgressCallback {
	// used to save the arguments in tool_call events, for use at tool_result time
	toolCallCache := make(map[string]map[string]interface{}) // toolCallId -> arguments
	skillCallCache := make(map[string]string)                // toolCallId -> skillName
	skillToolName := "skill"
	if h.config != nil {
		if customName := strings.TrimSpace(h.config.MultiAgent.EinoSkills.SkillToolName); customName != "" {
			skillToolName = customName
		}
	}

	extractSkillName := func(args map[string]interface{}) string {
		if len(args) == 0 {
			return ""
		}
		for _, key := range []string{"skill_name", "skillName", "name", "skill", "id", "skill_id", "skillId"} {
			if v, ok := args[key]; ok {
				switch vv := v.(type) {
				case string:
					if s := strings.TrimSpace(vv); s != "" {
						return s
					}
				case map[string]interface{}:
					for _, nestedKey := range []string{"name", "id", "skill_name", "skillId"} {
						if nestedV, nestedOK := vv[nestedKey].(string); nestedOK {
							if s := strings.TrimSpace(nestedV); s != "" {
								return s
							}
						}
					}
				}
			}
		}
		return ""
	}

	// thinking_stream_* (ReAct assistant body stream) and reasoning_chain_stream_* (Eino ReasoningContent):
	// not persisted per-item; aggregated by streamId, flushed as thinking / reasoning_chain.
	thinkingStreams := make(map[string]*thinkingBuf) // streamId -> buf
	flushedThinking := make(map[string]bool)         // streamId -> flushed
	seenToolCallSigs := make(map[string]string)      // toolCallId -> payload signature
	seenToolResultSigs := make(map[string]string)    // toolCallId -> payload signature

	// progressMu protects the map and aggregated state inside the closure. Eino parallelRunToolCall invokes callbacks concurrently across goroutines
	// progress (ToolInvokeNotifyHolder.Fire → createProgressCallback); an unlocked map would trigger a fatal panic.
	var progressMu sync.Mutex

	// response_start + response_delta: displayed as "📝 Planning" on the frontend timeline (monitor.js); individual deltas are not persisted;
	// aggregated into a single planning entry written to process_details, consistent with the live view after refresh.
	var respPlan responsePlanAgg
	if assistantMessageID != "" {
		h.tasks.SetHitlAssistantMessageID(conversationID, assistantMessageID)
	}
	syncHitlCognition := func() {
		h.syncHitlCognitionFromProgress(conversationID, assistantMessageID, thinkingStreams, &respPlan)
	}
	persistResponsePlan := func(reset bool) {
		if assistantMessageID == "" {
			return
		}
		content := strings.TrimSpace(respPlan.b.String())
		if content == "" {
			if reset {
				respPlan = responsePlanAgg{}
			}
			return
		}
		data := map[string]interface{}{
			"source": "response_stream",
		}
		for k, v := range respPlan.meta {
			data[k] = v
		}
		var err error
		if respPlan.detailID == "" {
			respPlan.detailID, err = h.db.AddProcessDetailWithID(
				assistantMessageID, conversationID, "planning", content, data,
			)
		} else {
			err = h.db.UpdateProcessDetailContent(respPlan.detailID, content, data)
		}
		if err != nil {
			h.logger.Warn("failed to save process details", zap.Error(err), zap.String("eventType", "planning"))
		} else {
			respPlan.lastPersistAt = time.Now()
			respPlan.lastPersistSize = respPlan.b.Len()
		}
		syncHitlCognition()
		if reset {
			respPlan = responsePlanAgg{}
		}
	}
	flushResponsePlan := func() { persistResponsePlan(true) }

	flushThinkingStreams := func() {
		if assistantMessageID == "" {
			return
		}
		for sid, tb := range thinkingStreams {
			if sid == "" || flushedThinking[sid] || tb == nil {
				continue
			}
			content := strings.TrimSpace(tb.b.String())
			if content == "" {
				flushedThinking[sid] = true
				continue
			}
			data := map[string]interface{}{
				"streamId": sid,
			}
			for k, v := range tb.meta {
				// avoid overwriting streamId
				if k == "streamId" {
					continue
				}
				data[k] = v
			}
			persist := tb.persistAs
			if persist != "reasoning_chain" {
				persist = "thinking"
			}
			if err := h.db.AddProcessDetail(assistantMessageID, conversationID, persist, content, data); err != nil {
				h.logger.Warn("failed to save process details", zap.Error(err), zap.String("eventType", persist))
			}
			flushedThinking[sid] = true
		}
		syncHitlCognition()
	}

	return func(eventType, message string, data interface{}) {
		progressMu.Lock()
		defer progressMu.Unlock()

		if isInternalEinoDiagnosticProgress(eventType, message, data) {
			return
		}

		// Upstream may re-invoke the same tool_call/tool_result during retries/compensation.
		// Apply idempotency filtering here to ensure both frontend display and process_details are based on unique events.
		if (eventType == "tool_call" || eventType == "tool_result") && data != nil {
			if dataMap, ok := data.(map[string]interface{}); ok {
				toolCallID := strings.TrimSpace(fmt.Sprint(dataMap["toolCallId"]))
				if toolCallID != "" && toolCallID != "<nil>" {
					payloadJSON, _ := json.Marshal(dataMap)
					sig := eventType + "|" + message + "|" + string(payloadJSON)
					seen := seenToolCallSigs
					if eventType == "tool_result" {
						seen = seenToolResultSigs
					}
					if prev, exists := seen[toolCallID]; exists && prev == sig {
						h.logger.Debug("skipping duplicate tool progress event",
							zap.String("eventType", eventType),
							zap.String("toolCallId", toolCallID))
						return
					}
					seen[toolCallID] = sig
				}
			}
		}

		// tool output fragments are not displayed in real-time in the details area; full results are fetched on demand after tool_result is persisted.
		if eventType == "tool_result_delta" {
			return
		}

		deferToolProgressSend := eventType == "tool_call" || eventType == "tool_result"
		// The main HTTP SSE and taskEventBus must both be written: page refresh severs the original connection, and after refresh
		// GET task-events subscription depends on eventBus to continue receiving subsequent iterations. Bots etc. without main SSE
		// the source also only writes to eventBus. Tool events must be persisted first to get processDetailId, then send summary.
		if !deferToolProgressSend {
			clientData := enrichProgressEventData(data, conversationID, assistantMessageID)
			if sendEventFunc != nil {
				sendEventFunc(eventType, message, clientData)
			}
			h.publishProgressToTaskEventBus(conversationID, eventType, message, clientData)
		}

		// save arguments from tool_call event
		if eventType == "tool_call" {
			if dataMap, ok := data.(map[string]interface{}); ok {
				toolName, _ := dataMap["toolName"].(string)
				if toolName == builtin.ToolSearchKnowledgeBase {
					if toolCallId, ok := dataMap["toolCallId"].(string); ok && toolCallId != "" {
						if argumentsObj, ok := dataMap["argumentsObj"].(map[string]interface{}); ok {
							toolCallCache[toolCallId] = argumentsObj
						}
					}
				}
				if strings.EqualFold(strings.TrimSpace(toolName), skillToolName) {
					toolCallID, _ := dataMap["toolCallId"].(string)
					if toolCallID != "" {
						if argumentsObj, ok := dataMap["argumentsObj"].(map[string]interface{}); ok {
							if skillName := extractSkillName(argumentsObj); skillName != "" {
								skillCallCache[toolCallID] = skillName
							}
						}
					}
				}
			}
		}

		if eventType == "tool_result" {
			if dataMap, ok := data.(map[string]interface{}); ok {
				toolName, _ := dataMap["toolName"].(string)
				toolCallID, _ := dataMap["toolCallId"].(string)
				success := true
				if v, ok := dataMap["success"].(bool); ok {
					success = v
				}
				resultText := ""
				if r, ok := dataMap["result"].(string); ok {
					resultText = r
				}
				if strings.TrimSpace(resultText) == "" {
					resultText = message
				}
				h.recordHitlToolExecutionResult(conversationID, toolCallID, toolName, success, resultText)
			}
		}

		// handle knowledge retrieval log recording
		if eventType == "tool_result" && h.knowledgeManager != nil {
			if dataMap, ok := data.(map[string]interface{}); ok {
				toolName, _ := dataMap["toolName"].(string)
				if toolName == builtin.ToolSearchKnowledgeBase {
					// extract retrieval info
					query := ""
					riskType := ""
					var retrievedItems []string

					// first try to get parameters from tool_call cache
					if toolCallId, ok := dataMap["toolCallId"].(string); ok && toolCallId != "" {
						if cachedArgs, exists := toolCallCache[toolCallId]; exists {
							if q, ok := cachedArgs["query"].(string); ok && q != "" {
								query = q
							}
							if rt, ok := cachedArgs["risk_type"].(string); ok && rt != "" {
								riskType = rt
							}
							// clean up cache after use
							delete(toolCallCache, toolCallId)
						}
					}

					// if not in cache, try to extract from argumentsObj
					if query == "" {
						if arguments, ok := dataMap["argumentsObj"].(map[string]interface{}); ok {
							if q, ok := arguments["query"].(string); ok && q != "" {
								query = q
							}
							if rt, ok := arguments["risk_type"].(string); ok && rt != "" {
								riskType = rt
							}
						}
					}

					// if query is still empty, try to extract from result (from the first line of the result text)
					if query == "" {
						if result, ok := dataMap["result"].(string); ok && result != "" {
							// try to extract query content from result (if result contains "no knowledge related to query 'xxx' found")
							if strings.Contains(result, "no knowledge related to query '") {
								start := strings.Index(result, "no knowledge related to query '") + len("no knowledge related to query '")
								end := strings.Index(result[start:], "'")
								if end > 0 {
									query = result[start : start+end]
								}
							}
						}
						// if still empty, use default value
						if query == "" {
							query = "unknown query"
						}
					}

					// extract retrieved knowledge item IDs from tool result
					// result format: "Found X relevant knowledge items:\n\n--- Result 1 (similarity: XX.XX%) ---\nSource: [category] title\n...\n<!-- METADATA: {...} -->"
					if result, ok := dataMap["result"].(string); ok && result != "" {
						// try to extract knowledge item ID from metadata
						metadataMatch := strings.Index(result, "<!-- METADATA:")
						if metadataMatch > 0 {
							// extract metadata JSON
							metadataStart := metadataMatch + len("<!-- METADATA: ")
							metadataEnd := strings.Index(result[metadataStart:], " -->")
							if metadataEnd > 0 {
								metadataJSON := result[metadataStart : metadataStart+metadataEnd]
								var metadata map[string]interface{}
								if err := json.Unmarshal([]byte(metadataJSON), &metadata); err == nil {
									if meta, ok := metadata["_metadata"].(map[string]interface{}); ok {
										if ids, ok := meta["retrievedItemIDs"].([]interface{}); ok {
											retrievedItems = make([]string, 0, len(ids))
											for _, id := range ids {
												if idStr, ok := id.(string); ok {
													retrievedItems = append(retrievedItems, idStr)
												}
											}
										}
									}
								}
							}
						}

						// if none extracted from metadata but result contains "Found X items", at least mark as having results
						if len(retrievedItems) == 0 && strings.Contains(result, "Found") && !strings.Contains(result, "not found") {
							// has results but cannot extract ID accurately; use a special marker
							retrievedItems = []string{"_has_results"}
						}
					}

					// record retrieval log (async, non-blocking)
					go func() {
						if err := h.knowledgeManager.LogRetrieval(conversationID, assistantMessageID, query, riskType, retrievedItems); err != nil {
							h.logger.Warn("failed to record knowledge retrieval log", zap.Error(err))
						}
					}()

					// add knowledge retrieval event to processDetails
					if assistantMessageID != "" {
						retrievalData := map[string]interface{}{
							"query":    query,
							"riskType": riskType,
							"toolName": toolName,
						}
						if err := h.db.AddProcessDetail(assistantMessageID, conversationID, "knowledge_retrieval", fmt.Sprintf("knowledge retrieval: %s", query), retrievalData); err != nil {
							h.logger.Warn("save knowledge retrieval details failed", zap.Error(err))
						}
					}
				}
			}
		}

		// record skills call statistics (tool_call + tool_result association)
		if eventType == "tool_result" && h.db != nil {
			if dataMap, ok := data.(map[string]interface{}); ok {
				toolName, _ := dataMap["toolName"].(string)
				if strings.EqualFold(strings.TrimSpace(toolName), skillToolName) {
					toolCallID, _ := dataMap["toolCallId"].(string)
					skillName := ""
					if toolCallID != "" {
						skillName = strings.TrimSpace(skillCallCache[toolCallID])
						delete(skillCallCache, toolCallID)
					}
					if skillName == "" {
						if argumentsObj, ok := dataMap["argumentsObj"].(map[string]interface{}); ok {
							skillName = strings.TrimSpace(extractSkillName(argumentsObj))
						}
					}
					if skillName != "" {
						success, ok := dataMap["success"].(bool)
						if !ok {
							if isError, okErr := dataMap["isError"].(bool); okErr {
								success = !isError
							}
						}
						successCalls := 0
						failedCalls := 0
						if success {
							successCalls = 1
						} else {
							failedCalls = 1
						}
						now := time.Now()
						if err := h.db.UpdateSkillStats(skillName, 1, successCalls, failedCalls, &now); err != nil {
							h.logger.Warn("failed to update skills call statistics", zap.Error(err), zap.String("skill", skillName))
						}
					}
				}
			}
		}

		// sub-agent reply streaming deltas are not persisted; merged into a single eino_agent_reply at the end
		if assistantMessageID != "" && eventType == "eino_agent_reply_stream_end" {
			flushResponsePlan()
			// ensure the thinking stream can be persisted before the sub-agent reply (readable after refresh)
			flushThinkingStreams()
			if err := h.db.AddProcessDetail(assistantMessageID, conversationID, "eino_agent_reply", message, data); err != nil {
				h.logger.Warn("failed to save process details", zap.Error(err), zap.String("eventType", eventType))
			}
			return
		}

		// multi-agent primary agent "planning": response_start / response_delta are for SSE only; aggregated into a single planning entry
		if eventType == "response_start" {
			if dataMap, ok := data.(map[string]interface{}); ok {
				if sameResponseStreamMeta(respPlan.meta, dataMap) {
					if respPlan.meta == nil {
						respPlan.meta = make(map[string]interface{}, len(dataMap))
					}
					for k, v := range dataMap {
						respPlan.meta[k] = v
					}
					return
				}
			}
			flushResponsePlan()
			// reasoning stream usually ends before assistant body starts; persisted so 'pentest details' can be replayed after refresh
			flushThinkingStreams()
			respPlan.meta = nil
			if dataMap, ok := data.(map[string]interface{}); ok {
				respPlan.meta = make(map[string]interface{}, len(dataMap))
				for k, v := range dataMap {
					respPlan.meta[k] = v
				}
			}
			respPlan.b.Reset()
			return
		}
		if eventType == "response_delta" {
			if dataMap, ok := data.(map[string]interface{}); ok {
				if acc, okAcc := dataMap[openai.SSEAccumulatedKey].(string); okAcc {
					respPlan.b.Reset()
					respPlan.b.WriteString(acc)
				} else {
					respPlan.b.WriteString(message)
				}
			} else {
				respPlan.b.WriteString(message)
			}
			if dataMap, ok := data.(map[string]interface{}); ok && respPlan.meta == nil {
				respPlan.meta = make(map[string]interface{}, len(dataMap))
				for k, v := range dataMap {
					respPlan.meta[k] = v
				}
			} else if dataMap, ok := data.(map[string]interface{}); ok {
				for k, v := range dataMap {
					respPlan.meta[k] = v
				}
			}
			// the running primary reply cannot be stored in memory only: page refresh destroys the old page, and new task-events
			// subscriptions can only receive future increments. Throttle updates to the same planning record by time or delta size,
			// so that on refresh the full text already displayed before the refresh can be resumed from the database.
			if respPlan.lastPersistAt.IsZero() ||
				time.Since(respPlan.lastPersistAt) >= 300*time.Millisecond ||
				respPlan.b.Len()-respPlan.lastPersistSize >= 1024 {
				persistResponsePlan(false)
			}
			syncHitlCognition()
			return
		}
		if eventType == "response" {
			flushResponsePlan()
			flushThinkingStreams()
			return
		}
		if eventType == "done" {
			flushResponsePlan()
			flushThinkingStreams()
			return
		}

		// streaming thinking/reasoning ended: aggregate and persist (same logic as eino_agent_reply_stream_end)
		if eventType == "thinking_stream_end" || eventType == "reasoning_chain_stream_end" {
			flushResponsePlan()
			flushThinkingStreams()
			return
		}

		// aggregate thinking_stream_* / reasoning_chain_stream_*; do not persist per-item
		if eventType == "thinking_stream_start" || eventType == "reasoning_chain_stream_start" {
			persistAs := "thinking"
			if eventType == "reasoning_chain_stream_start" {
				persistAs = "reasoning_chain"
			}
			if dataMap, ok := data.(map[string]interface{}); ok {
				if sid, ok2 := dataMap["streamId"].(string); ok2 && sid != "" {
					tb := thinkingStreams[sid]
					if tb == nil {
						tb = &thinkingBuf{meta: map[string]interface{}{}, persistAs: persistAs}
						thinkingStreams[sid] = tb
					} else {
						tb.persistAs = persistAs
					}
					// record meta info (source/einoAgent/einoRole/iteration etc.)
					for k, v := range dataMap {
						tb.meta[k] = v
					}
				}
			}
			return
		}
		if eventType == "thinking_stream_delta" || eventType == "reasoning_chain_stream_delta" {
			persistAs := "thinking"
			if eventType == "reasoning_chain_stream_delta" {
				persistAs = "reasoning_chain"
			}
			if dataMap, ok := data.(map[string]interface{}); ok {
				if sid, ok2 := dataMap["streamId"].(string); ok2 && sid != "" {
					tb := thinkingStreams[sid]
					if tb == nil {
						tb = &thinkingBuf{meta: map[string]interface{}{}, persistAs: persistAs}
						thinkingStreams[sid] = tb
					} else if tb.persistAs == "" {
						tb.persistAs = persistAs
					}
					if acc, okAcc := dataMap[openai.SSEAccumulatedKey].(string); okAcc {
						tb.b.Reset()
						tb.b.WriteString(acc)
					} else {
						tb.b.WriteString(message)
					}
					// sometimes delta arrives before start; supplement meta info
					for k, v := range dataMap {
						tb.meta[k] = v
					}
				}
			}
			syncHitlCognition()
			return
		}

		// when the Agent sends both *_stream_* and thinking/reasoning_chain with the same streamId,
		// the streaming aggregation will already persist via flushThinkingStreams(); skip per-item duplication here.
		if eventType == "thinking" || eventType == "reasoning_chain" {
			if dataMap, ok := data.(map[string]interface{}); ok {
				if sid, ok2 := dataMap["streamId"].(string); ok2 && sid != "" {
					if tb, exists := thinkingStreams[sid]; exists && tb != nil {
						if strings.TrimSpace(tb.b.String()) != "" {
							return
						}
					}
					if flushedThinking[sid] {
						return
					}
				}
			}
		}

		// save process details to database (excluding response/done; response body is already in messages table)
		// response_start/response_delta have been aggregated into planning; do not persist per-item.
		// [Eino] agent heartbeat progress is for real-time progress title only; not persisted to avoid cluttering the timeline.
		skipEinoAgentHeartbeat := eventType == "progress" && strings.HasPrefix(strings.TrimSpace(message), "[Eino] ")
		if assistantMessageID != "" &&
			!skipEinoAgentHeartbeat &&
			eventType != "response" &&
			eventType != "done" &&
			eventType != "response_start" &&
			eventType != "response_delta" &&
			eventType != "tool_result_delta" &&
			eventType != "eino_trace_run" &&
			eventType != "eino_trace_start" &&
			eventType != "eino_trace_end" &&
			eventType != "eino_trace_error" &&
			eventType != "eino_agent_reply_stream_start" &&
			eventType != "eino_agent_reply_stream_delta" &&
			eventType != "eino_agent_reply_stream_end" {
			if eventType == "tool_result" {
				if detailID := discardPlanningIfEchoesToolResult(&respPlan, data); detailID != "" {
					if err := h.db.DeleteProcessDetail(detailID); err != nil {
						h.logger.Warn("failed to delete tool result echo plan", zap.Error(err), zap.String("processDetailId", detailID))
					}
				}
			}
			// before persisting key process events, first flush the "planning" and in-progress thinking / reasoning_chain streams
			flushResponsePlan()
			flushThinkingStreams()
			processDetailID, err := h.db.AddProcessDetailWithID(assistantMessageID, conversationID, eventType, message, data)
			if err != nil {
				h.logger.Warn("failed to save process details", zap.Error(err), zap.String("eventType", eventType))
			}
			if deferToolProgressSend {
				clientData := enrichProgressEventData(summarizeProcessDetailData(eventType, data), conversationID, assistantMessageID)
				if m, ok := clientData.(map[string]interface{}); ok {
					m["processDetailId"] = processDetailID
				}
				if sendEventFunc != nil {
					sendEventFunc(eventType, message, clientData)
				}
				h.publishProgressToTaskEventBus(conversationID, eventType, message, clientData)
			}
		} else if deferToolProgressSend {
			clientData := enrichProgressEventData(summarizeProcessDetailData(eventType, data), conversationID, assistantMessageID)
			if sendEventFunc != nil {
				sendEventFunc(eventType, message, clientData)
			}
			h.publishProgressToTaskEventBus(conversationID, eventType, message, clientData)
		}
	}
}

// cancelToolContinueAfter terminates only the current tool call, does not stop the entire Agent task (shared by conversation 'interrupt and continue' and MCP monitor termination).
func (h *AgentHandler) cancelToolContinueAfter(conversationID, preferredExecID, note string) (bool, gin.H) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" || h.tasks.GetTask(conversationID) == nil {
		return false, nil
	}
	note = strings.TrimSpace(note)
	execID := strings.TrimSpace(preferredExecID)
	if execID == "" {
		execID = h.tasks.ActiveMCPExecutionID(conversationID)
	}
	if execID != "" {
		if h.agent.CancelMCPToolExecutionWithNote(execID, note) {
			return true, gin.H{
				"status":              "tool_abort_requested",
				"conversationId":      conversationID,
				"executionId":         execID,
				"message":             "Current tool call termination requested; reasoning will continue after tool returns (consistent with MCP monitor page termination).",
				"continueAfter":       true,
				"interruptWithNote":   note != "",
				"continueWithoutTool": false,
			}
		}
		if h.tasks.AbortActiveEinoExecute(conversationID, note) {
			return true, gin.H{
				"status":              "tool_abort_requested",
				"conversationId":      conversationID,
				"executionId":         execID,
				"message":             "Current execute command termination requested; reasoning will continue after command returns.",
				"continueAfter":       true,
				"interruptWithNote":   note != "",
				"continueWithoutTool": false,
			}
		}
		return false, nil
	}
	if h.tasks.AbortActiveEinoExecute(conversationID, note) {
		return true, gin.H{
			"status":              "tool_abort_requested",
			"conversationId":      conversationID,
			"message":             "Current execute command termination requested; reasoning will continue after command returns.",
			"continueAfter":       true,
			"interruptWithNote":   note != "",
			"continueWithoutTool": false,
		}
	}
	return false, nil
}

// CancelAgentLoop cancels the currently executing task
func (h *AgentHandler) CancelAgentLoop(c *gin.Context) {
	var req struct {
		ConversationID string `json:"conversationId" binding:"required"`
		ExecutionID    string `json:"executionId,omitempty"`
		Reason         string `json:"reason,omitempty"`
		ContinueAfter  bool   `json:"continueAfter,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.agentConversationAllowed(c, req.ConversationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}

	if req.ContinueAfter {
		if h.tasks.GetTask(req.ConversationID) == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "no running task found"})
			return
		}
		note := strings.TrimSpace(req.Reason)
		activeExec := strings.TrimSpace(h.tasks.ActiveMCPExecutionID(req.ConversationID))
		if ok, payload := h.cancelToolContinueAfter(req.ConversationID, strings.TrimSpace(req.ExecutionID), note); ok {
			execID, _ := payload["executionId"].(string)
			h.logger.Info("conversation page: terminating current tool only",
				zap.String("conversationId", req.ConversationID),
				zap.String("executionId", execID),
				zap.Bool("hasNote", note != ""),
			)
			c.JSON(http.StatusOK, payload)
			return
		}
		if activeExec != "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "no in-progress tool execution found or the call has already ended"})
			return
		}
		// no in-progress MCP tool (model pure inference/streaming output phase): cancel the current context and let the Eino streaming handler merge the user supplement then auto-continue.
		h.tasks.SetInterruptContinueNote(req.ConversationID, note)
		ok, err := h.tasks.CancelTask(req.ConversationID, multiagent.ErrInterruptContinue)
		if err != nil {
			h.logger.Error("interrupt and continue (no tool) failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "no running task found"})
			return
		}
		h.logger.Info("conversation page: interrupt and continue (no MCP tool; will auto-continue)",
			zap.String("conversationId", req.ConversationID),
			zap.Bool("hasNote", note != ""),
		)
		c.JSON(http.StatusOK, gin.H{
			"status":              "interrupt_continue_scheduled",
			"conversationId":      req.ConversationID,
			"message":             "Pause requested for current reasoning; user supplement will be merged into context and execution will auto-continue (no full round stop needed).",
			"continueAfter":       true,
			"interruptWithNote":   note != "",
			"continueWithoutTool": true,
		})
		return
	}

	var cause error = ErrTaskCancelled
	msg := "Cancellation request submitted; task will stop after the current step completes."
	h.cancelRunningMCPToolsForConversation(req.ConversationID)
	h.tasks.AbortActiveEinoExecute(req.ConversationID, "")
	ok, err := h.tasks.CancelTask(req.ConversationID, cause)
	if err != nil {
		h.logger.Error("cancelled task failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no running task found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":            "cancelling",
		"conversationId":    req.ConversationID,
		"message":           msg,
		"continueAfter":     false,
		"interruptWithNote": false,
	})
}

// SubscribeAgentTaskEvents is GET SSE: subscribe to a mirror of the current running task events for the specified conversation (frame format identical to POST .../stream), used for page refresh or reconnecting after disconnect.
func (h *AgentHandler) SubscribeAgentTaskEvents(c *gin.Context) {
	conversationID := strings.TrimSpace(c.Query("conversationId"))
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversationId is required"})
		return
	}
	if !h.agentConversationAllowed(c, conversationID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied for this resource"})
		return
	}
	if h.tasks.GetTask(conversationID) == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active task for this conversation"})
		return
	}
	if h.taskEventBus == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "task event bus unavailable"})
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	sub, ch := h.taskEventBus.Subscribe(conversationID)
	defer h.taskEventBus.Unsubscribe(conversationID, sub)

	flusher, _ := c.Writer.(http.Flusher)
	ctx := c.Request.Context()
	var writeMu sync.Mutex
	stopKeepalive := runSSEKeepalive(c, &writeMu)
	defer stopKeepalive()

	for {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-ch:
			if !ok {
				return
			}
			writeMu.Lock()
			if _, err := c.Writer.Write(chunk); err != nil {
				writeMu.Unlock()
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
			writeMu.Unlock()
		}
	}
}

// enrichAgentTasksWithConversationTitles appends the current session title to the task list (for top bar/task page display; auto-synced after rename)
func (h *AgentHandler) enrichAgentTasksWithConversationTitles(tasks []*AgentTask) {
	if h == nil || h.db == nil {
		return
	}
	for _, task := range tasks {
		if task == nil || strings.TrimSpace(task.ConversationID) == "" {
			continue
		}
		if title, err := h.db.GetConversationTitle(task.ConversationID); err == nil {
			task.Title = strings.TrimSpace(title)
		}
	}
}

// enrichCompletedTasksWithConversationTitles appends the current session title to completed tasks
func (h *AgentHandler) enrichCompletedTasksWithConversationTitles(tasks []*CompletedTask) {
	if h == nil || h.db == nil {
		return
	}
	for _, task := range tasks {
		if task == nil || strings.TrimSpace(task.ConversationID) == "" {
			continue
		}
		if title, err := h.db.GetConversationTitle(task.ConversationID); err == nil {
			task.Title = strings.TrimSpace(title)
		}
	}
}

// ListAgentTasks lists all running tasks
func (h *AgentHandler) ListAgentTasks(c *gin.Context) {
	tasks := h.tasks.GetActiveTasks()
	tasks = filterSlice(tasks, func(task *AgentTask) bool {
		return task != nil && h.agentConversationAllowed(c, task.ConversationID)
	})
	h.enrichAgentTasksWithConversationTitles(tasks)
	c.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
	})
}

// ListCompletedTasks lists recently completed task history
func (h *AgentHandler) ListCompletedTasks(c *gin.Context) {
	tasks := h.tasks.GetCompletedTasks()
	tasks = filterSlice(tasks, func(task *CompletedTask) bool {
		return task != nil && h.agentConversationAllowed(c, task.ConversationID)
	})
	h.enrichCompletedTasksWithConversationTitles(tasks)
	c.JSON(http.StatusOK, gin.H{
		"tasks": tasks,
	})
}

func (h *AgentHandler) agentConversationAllowed(c *gin.Context, conversationID string) bool {
	session, ok := security.CurrentSession(c)
	return ok && h.db != nil && h.db.UserCanAccessResource(session.UserID, session.Scope, "conversation", strings.TrimSpace(conversationID))
}

func filterSlice[T any](items []T, keep func(T) bool) []T {
	out := make([]T, 0, len(items))
	for _, item := range items {
		if keep(item) {
			out = append(out, item)
		}
	}
	return out
}

// BatchTaskRequest is the batch task request
type BatchTaskRequest struct {
	HITLPolicy   string   `json:"hitlPolicy"`
	Title        string   `json:"title"`                    // Task title (optional)
	Tasks        []string `json:"tasks" binding:"required"` // Task list, one task per line
	Role         string   `json:"role,omitempty"`           // role name (optional; empty string means default role)
	AgentMode    string   `json:"agentMode,omitempty"`      // eino_single | deep | plan_execute | supervisor
	ScheduleMode string   `json:"scheduleMode,omitempty"`   // manual | cron
	CronExpr     string   `json:"cronExpr,omitempty"`       // required when scheduleMode=cron
	ExecuteNow   bool     `json:"executeNow,omitempty"`     // whether to execute immediately after creation (default false)
	ProjectID    string   `json:"projectId,omitempty"`      // project bound to sub-conversations in the queue (optional)
	Concurrency  int      `json:"concurrency,omitempty"`    // number of sub-tasks to execute concurrently, default 1, max 8
}

// batchQueueWantsEino returns whether the queue is configured to use Eino multi-agent.
func batchQueueWantsEino(agentMode string) bool {
	m := strings.TrimSpace(strings.ToLower(agentMode))
	return m == "deep" || m == "plan_execute" || m == "supervisor"
}

func normalizeBatchQueueScheduleMode(mode string) string {
	if strings.TrimSpace(mode) == "cron" {
		return "cron"
	}
	return "manual"
}

// CreateBatchQueue creates a batch task queue
func (h *AgentHandler) CreateBatchQueue(c *gin.Context) {
	var req BatchTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.Tasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task list cannot be empty"})
		return
	}

	// filternulltask
	validTasks := make([]string, 0, len(req.Tasks))
	for _, task := range req.Tasks {
		if task != "" {
			validTasks = append(validTasks, task)
		}
	}

	if len(validTasks) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid tasks"})
		return
	}
	if session, ok := security.CurrentSession(c); ok && h.db != nil && session.Scope != database.RBACScopeAll && strings.TrimSpace(req.ProjectID) != "" {
		if !h.db.UserCanAccessResource(session.UserID, session.Scope, "project", strings.TrimSpace(req.ProjectID)) {
			c.JSON(http.StatusForbidden, gin.H{"error": "no permission to create batch tasks under this project"})
			return
		}
	}

	agentMode := config.NormalizeAgentMode(req.AgentMode)
	scheduleMode := normalizeBatchQueueScheduleMode(req.ScheduleMode)
	cronExpr := strings.TrimSpace(req.CronExpr)
	var nextRunAt *time.Time
	if scheduleMode == "cron" {
		if cronExpr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cron expression cannot be empty when enabling Cron scheduling"})
			return
		}
		schedule, err := h.batchCronParser.Parse(cronExpr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Cron expression: " + err.Error()})
			return
		}
		next := schedule.Next(time.Now())
		nextRunAt = &next
	}

	queue, createErr := h.batchTaskManager.CreateBatchQueue(req.Title, req.Role, agentMode, scheduleMode, cronExpr, req.ProjectID, nextRunAt, req.Concurrency, validTasks, req.HITLPolicy)
	if createErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": createErr.Error()})
		return
	}
	if session, ok := security.CurrentSession(c); ok && h.db != nil {
		_ = h.db.SetResourceOwner("batch_task", queue.ID, session.UserID)
		_ = h.db.AssignResourceToUser(session.UserID, "batch_task", queue.ID)
	}
	started := false
	if req.ExecuteNow {
		ok, err := h.startBatchQueueExecution(queue.ID, false)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "queueId": queue.ID})
			return
		}
		started = true
		if refreshed, exists := h.batchTaskManager.GetBatchQueue(queue.ID); exists {
			queue = refreshed
		}
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "task", "create_queue", "create batch task queue", "batch_queue", queue.ID, map[string]interface{}{
			"task_count": len(validTasks), "started": started,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"queueId": queue.ID,
		"queue":   queue,
		"started": started,
	})
}

// GetBatchQueue gets a batch task queue
func (h *AgentHandler) GetBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"queue": queue})
}

// ListBatchQueuesResponse is the batch task queue list response
type ListBatchQueuesResponse struct {
	Queues     []*BatchTaskQueue `json:"queues"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"page_size"`
	TotalPages int               `json:"total_pages"`
}

// ListBatchQueues lists all batch task queues (supports filtering and pagination)
func (h *AgentHandler) ListBatchQueues(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "10")
	offsetStr := c.DefaultQuery("offset", "0")
	pageStr := c.Query("page")
	status := c.Query("status")
	keyword := c.Query("keyword")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)
	page := 1

	// if page parameter is provided, use it to calculate offset
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
			offset = (page - 1) * limit
		}
	}

	// limit pageSize range
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	// prevent maliciously large offset from causing DB performance issues
	const maxOffset = 100000
	if offset > maxOffset {
		offset = maxOffset
	}

	// default status is "all"
	if status == "" {
		status = "all"
	}

	// get queue list and total
	session, _ := security.CurrentSession(c)
	queues, total, err := h.batchTaskManager.ListQueuesForAccess(limit, offset, status, keyword, session.UserID, session.Scope)
	if err != nil {
		h.logger.Error("get batch task queue list failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// calculate total pages
	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	// if using offset to calculate page, need to recalculate
	if pageStr == "" {
		page = (offset / limit) + 1
	}

	response := ListBatchQueuesResponse{
		Queues:     queues,
		Total:      total,
		Page:       page,
		PageSize:   limit,
		TotalPages: totalPages,
	}

	c.JSON(http.StatusOK, response)
}

// StartBatchQueue starts executing the batch task queue
func (h *AgentHandler) StartBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	h.batchTaskManager.ClearSingleRunTask(queueID)
	ok, err := h.startBatchQueueExecution(queueID, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "task", "start_queue", "start batch task queue", "batch_queue", queueID, nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "Batch tasks started", "queueId": queueID})
}

// RerunBatchQueue re-runs the batch task queue (resets all sub-tasks then re-executes)
func (h *AgentHandler) RerunBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	if queue.Status != "completed" && queue.Status != "cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only completed or cancelled queues can be re-run"})
		return
	}
	if !h.batchTaskManager.ResetQueueForRerun(queueID) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "reset queue failed"})
		return
	}
	h.batchTaskManager.ClearSingleRunTask(queueID)
	ok, err := h.startBatchQueueExecution(queueID, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "startup failed"})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "task", "rerun_queue", "re-run batch task queue", "batch_queue", queueID, nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "batch task has resumed execution", "queueId": queueID})
}

// PauseBatchQueue pauses the batch task queue
func (h *AgentHandler) PauseBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	success := h.batchTaskManager.PauseQueue(queueID)
	if !success {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found or cannot be paused"})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "task", "pause_queue", "pause batch task queue", "batch_queue", queueID, nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "Batch tasks paused"})
}

// UpdateBatchQueueMetadata updates the batch task queue title, role, and agent mode
func (h *AgentHandler) UpdateBatchQueueMetadata(c *gin.Context) {
	queueID := c.Param("queueId")
	var req struct {
		HITLPolicy  *string `json:"hitlPolicy"`
		Title       string  `json:"title"`
		Role        string  `json:"role"`
		AgentMode   string  `json:"agentMode"`
		Concurrency *int    `json:"concurrency"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var policies []string
	if req.HITLPolicy != nil {
		policies = append(policies, *req.HITLPolicy)
	}
	if err := h.batchTaskManager.UpdateQueueMetadata(queueID, req.Title, req.Role, req.AgentMode, req.Concurrency, policies...); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, _ := h.batchTaskManager.GetBatchQueue(queueID)
	c.JSON(http.StatusOK, gin.H{"queue": updated})
}

// UpdateBatchQueueSchedule updates the batch task queue scheduling config (scheduleMode / cronExpr)
func (h *AgentHandler) UpdateBatchQueueSchedule(c *gin.Context) {
	queueID := c.Param("queueId")
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	// only allow scheduling modification when not in running status
	if queue.Status == "running" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "queue is running, cannot modify scheduling config"})
		return
	}
	var req struct {
		ScheduleMode string `json:"scheduleMode"`
		CronExpr     string `json:"cronExpr"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	scheduleMode := normalizeBatchQueueScheduleMode(req.ScheduleMode)
	cronExpr := strings.TrimSpace(req.CronExpr)
	var nextRunAt *time.Time
	if scheduleMode == "cron" {
		if cronExpr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cron expression cannot be empty when enabling Cron scheduling"})
			return
		}
		schedule, err := h.batchCronParser.Parse(cronExpr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Cron expression: " + err.Error()})
			return
		}
		next := schedule.Next(time.Now())
		nextRunAt = &next
	}
	h.batchTaskManager.UpdateQueueSchedule(queueID, scheduleMode, cronExpr, nextRunAt)
	updated, _ := h.batchTaskManager.GetBatchQueue(queueID)
	c.JSON(http.StatusOK, gin.H{"queue": updated})
}

// SetBatchQueueScheduleEnabled enables/disables Cron automatic scheduling (manual execution is not affected)
func (h *AgentHandler) SetBatchQueueScheduleEnabled(c *gin.Context) {
	queueID := c.Param("queueId")
	if _, exists := h.batchTaskManager.GetBatchQueue(queueID); !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	var req struct {
		ScheduleEnabled bool `json:"scheduleEnabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.batchTaskManager.SetScheduleEnabled(queueID, req.ScheduleEnabled) {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	queue, _ := h.batchTaskManager.GetBatchQueue(queueID)
	c.JSON(http.StatusOK, gin.H{"queue": queue})
}

// DeleteBatchQueue deletes a batch task queue
func (h *AgentHandler) DeleteBatchQueue(c *gin.Context) {
	queueID := c.Param("queueId")
	if err := h.batchTaskManager.DeleteQueue(queueID); err != nil {
		switch {
		case errors.Is(err, ErrBatchQueueNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		case errors.Is(err, ErrBatchQueueExecutorActive):
			c.JSON(http.StatusConflict, gin.H{"error": "queue executor is still running, please delete later"})
		case errors.Is(err, ErrBatchQueueStillRunning):
			c.JSON(http.StatusConflict, gin.H{"error": "queue is running, cannot delete"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	if h.audit != nil {
		h.audit.Record(c, audit.Entry{
			Category:     "task",
			Action:       "delete_queue",
			Result:       "success",
			ResourceType: "batch_queue",
			ResourceID:   queueID,
			Message:      "delete batch task queue",
		})
	}
	c.JSON(http.StatusOK, gin.H{"message": "Batch task queue deleted"})
}

// UpdateBatchTask updates batch task message
func (h *AgentHandler) UpdateBatchTask(c *gin.Context) {
	queueID := c.Param("queueId")
	taskID := c.Param("taskId")

	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task message cannot be empty"})
		return
	}

	err := h.batchTaskManager.UpdateTaskMessage(queueID, taskID, req.Message)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// return updated queue info
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "task updated", "queue": queue})
}

// AddBatchTask adds a task to the batch task queue
func (h *AgentHandler) AddBatchTask(c *gin.Context) {
	queueID := c.Param("queueId")

	var req struct {
		Message string `json:"message" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request parameters: " + err.Error()})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task message cannot be empty"})
		return
	}

	task, err := h.batchTaskManager.AddTaskToQueue(queueID, req.Message)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// return updated queue info
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "task added", "task": task, "queue": queue})
}

// RunSingleBatchTask executes a single specified sub-task (can re-run already succeeded tasks), pauses queue after completion
func (h *AgentHandler) RunSingleBatchTask(c *gin.Context) {
	queueID := c.Param("queueId")
	taskID := c.Param("taskId")

	if err := h.batchTaskManager.PrepareSingleTaskRun(queueID, taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.batchTaskManager.SetSingleRunTask(queueID, taskID)

	// paused state single run: old batch goroutines may still occupy the execution slot, release them first so execution can restart
	if queue, ok := h.batchTaskManager.GetBatchQueue(queueID); ok && queue.Status == BatchQueueStatusPaused {
		h.batchTaskManager.ForceUnmarkQueueExecutor(queueID)
	}

	autoStarted := true
	autoStartMsg := "single task execution started"
	ok, startErr := h.startBatchQueueExecution(queueID, false)
	if startErr != nil {
		h.batchTaskManager.ClearSingleRunTask(queueID)
		autoStarted = false
		autoStartMsg = "task is ready but auto-start failed: " + startErr.Error()
	} else if !ok {
		h.batchTaskManager.ClearSingleRunTask(queueID)
		autoStarted = false
		autoStartMsg = "task is ready but queue does not exist"
	}

	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "task", "run_single_batch_task", "run single batch sub-task", "batch_task", taskID, map[string]interface{}{
			"batch_queue_id": queueID,
			"auto_started":   autoStarted,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"message":     autoStartMsg,
		"queue":       queue,
		"autoStarted": autoStarted,
	})
}

// DeleteBatchTask deletes a batch task
func (h *AgentHandler) DeleteBatchTask(c *gin.Context) {
	queueID := c.Param("queueId")
	taskID := c.Param("taskId")

	err := h.batchTaskManager.DeleteTask(queueID, taskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// return updated queue info
	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "queue not found"})
		return
	}
	if h.audit != nil {
		h.audit.RecordOK(c, "task", "delete_batch_task", "delete batch sub-task", "batch_task", taskID, map[string]interface{}{
			"batch_queue_id": queueID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"message": "taskdeleted", "queue": queue})
}

func (h *AgentHandler) nextBatchQueueRunAt(cronExpr string, from time.Time) (*time.Time, error) {
	expr := strings.TrimSpace(cronExpr)
	if expr == "" {
		return nil, nil
	}
	schedule, err := h.batchCronParser.Parse(expr)
	if err != nil {
		return nil, err
	}
	next := schedule.Next(from)
	return &next, nil
}

func (h *AgentHandler) startBatchQueueExecution(queueID string, scheduled bool) (bool, error) {
	// acquire execution mutex first, then read queue status to avoid decisions based on stale snapshots
	if !h.batchTaskManager.TryMarkQueueExecutor(queueID) {
		return true, nil
	}

	queue, exists := h.batchTaskManager.GetBatchQueue(queueID)
	if !exists {
		h.batchTaskManager.UnmarkQueueExecutor(queueID)
		return false, nil
	}

	if scheduled {
		if queue.ScheduleMode != "cron" {
			h.batchTaskManager.UnmarkQueueExecutor(queueID)
			err := fmt.Errorf("queue has cron scheduling disabled")
			h.batchTaskManager.SetLastScheduleError(queueID, err.Error())
			return true, err
		}
		if queue.Status == "running" || queue.Status == "paused" || queue.Status == "cancelled" {
			h.batchTaskManager.UnmarkQueueExecutor(queueID)
			err := fmt.Errorf("current queue status does not permit scheduled execution")
			h.batchTaskManager.SetLastScheduleError(queueID, err.Error())
			return true, err
		}
		if !h.batchTaskManager.ResetQueueForRerun(queueID) {
			h.batchTaskManager.UnmarkQueueExecutor(queueID)
			err := fmt.Errorf("reset queue failed")
			h.batchTaskManager.SetLastScheduleError(queueID, err.Error())
			return true, err
		}
		queue, _ = h.batchTaskManager.GetBatchQueue(queueID)
	} else if queue.Status != "pending" && queue.Status != "paused" {
		h.batchTaskManager.UnmarkQueueExecutor(queueID)
		return true, fmt.Errorf("queue status does not permit start")
	}

	if queue != nil && batchQueueWantsEino(queue.AgentMode) && (h.config == nil || !h.config.MultiAgent.Enabled) {
		h.batchTaskManager.UnmarkQueueExecutor(queueID)
		err := fmt.Errorf("current queue config is Eino multi-agent, but multi-agent is not enabled in system")
		if scheduled {
			h.batchTaskManager.SetLastScheduleError(queueID, err.Error())
		}
		return true, err
	}

	if scheduled {
		h.batchTaskManager.RecordScheduledRunStart(queueID)
	}
	h.batchTaskManager.UpdateQueueStatus(queueID, "running")
	if queue != nil && queue.ScheduleMode == "cron" {
		nextRunAt, err := h.nextBatchQueueRunAt(queue.CronExpr, time.Now())
		if err == nil {
			h.batchTaskManager.UpdateQueueSchedule(queueID, "cron", queue.CronExpr, nextRunAt)
		}
	}

	go h.executeBatchQueue(queueID)
	return true, nil
}

func (h *AgentHandler) batchQueueSchedulerLoop() {
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		queues := h.batchTaskManager.GetLoadedQueues()
		now := time.Now()
		for _, queue := range queues {
			if queue == nil || queue.ScheduleMode != "cron" || !queue.ScheduleEnabled || queue.Status == "cancelled" || queue.Status == "running" || queue.Status == "paused" {
				continue
			}
			nextRunAt := queue.NextRunAt
			if nextRunAt == nil {
				next, err := h.nextBatchQueueRunAt(queue.CronExpr, now)
				if err != nil {
					h.logger.Warn("batch task cron expression invalid, skipping schedule", zap.String("queueId", queue.ID), zap.String("cronExpr", queue.CronExpr), zap.Error(err))
					continue
				}
				h.batchTaskManager.UpdateQueueSchedule(queue.ID, "cron", queue.CronExpr, next)
				nextRunAt = next
			}
			if nextRunAt != nil && (nextRunAt.Before(now) || nextRunAt.Equal(now)) {
				if _, err := h.startBatchQueueExecution(queue.ID, true); err != nil {
					h.logger.Warn("auto-schedule batch task failed", zap.String("queueId", queue.ID), zap.Error(err))
				}
			}
		}
	}
}

// loadHistoryFromAgentTrace resumes history from the saved agent message trace in the database (columns last_react_*; includes single-agent and Eino).
// Logic mirrors attack chain: prefer the saved JSON message array + last-round assistant summary; fall back to the messages table if absent.
func (h *AgentHandler) loadHistoryFromAgentTrace(conversationID string) ([]agent.ChatMessage, error) {
	traceInputJSON, assistantOut, err := h.db.GetAgentTrace(conversationID)
	if err != nil {
		return nil, fmt.Errorf("get agent trace failed: %w", err)
	}

	if traceInputJSON == "" {
		return nil, fmt.Errorf("agent trace is empty, will use messages table")
	}

	dataSource := "database_last_agent_trace"

	var messagesArray []map[string]interface{}
	if err := json.Unmarshal([]byte(traceInputJSON), &messagesArray); err != nil {
		return nil, fmt.Errorf("parse agent trace JSON failed: %w", err)
	}

	messageCount := len(messagesArray)
	modelFacingTrace := agent.IsModelFacingTraceJSON(traceInputJSON)

	h.logger.Info("using saved agent trace to resume history context",
		zap.String("conversationId", conversationID),
		zap.String("dataSource", dataSource),
		zap.Int("traceInputSize", len(traceInputJSON)),
		zap.Int("messageCount", messageCount),
		zap.Int("assistantOutSize", len(assistantOut)),
	)

	// convert to Agent message format
	agentMessages := make([]agent.ChatMessage, 0, len(messagesArray))
	for _, msgMap := range messagesArray {
		msg := agent.ChatMessage{}
		msg.ModelFacingTrace = modelFacingTrace

		// parse role
		if role, ok := msgMap["role"].(string); ok {
			msg.Role = role
		} else {
			continue // skip invalid message
		}

		// skip system messages (provided by Eino Instruction)
		if msg.Role == "system" {
			continue
		}

		// parse content
		if content, ok := msgMap["content"].(string); ok {
			msg.Content = content
		}
		// DeepSeek thinking mode: assistant with tool calls must return reasoning_content in subsequent requests
		if rc, ok := msgMap["reasoning_content"].(string); ok && strings.TrimSpace(rc) != "" {
			msg.ReasoningContent = rc
		}

		// parse tool_calls (if present)
		if toolCallsRaw, ok := msgMap["tool_calls"]; ok && toolCallsRaw != nil {
			if toolCallsArray, ok := toolCallsRaw.([]interface{}); ok {
				msg.ToolCalls = make([]agent.ToolCall, 0, len(toolCallsArray))
				for _, tcRaw := range toolCallsArray {
					if tcMap, ok := tcRaw.(map[string]interface{}); ok {
						toolCall := agent.ToolCall{}

						// parse ID
						if id, ok := tcMap["id"].(string); ok {
							toolCall.ID = id
						}

						// parse Type
						if toolType, ok := tcMap["type"].(string); ok {
							toolCall.Type = toolType
						}

						// parse Function
						if funcMap, ok := tcMap["function"].(map[string]interface{}); ok {
							toolCall.Function = agent.FunctionCall{}

							// parse function name
							if name, ok := funcMap["name"].(string); ok {
								toolCall.Function.Name = name
							}

							// parse arguments (may be string or object)
							if argsRaw, ok := funcMap["arguments"]; ok {
								if argsStr, ok := argsRaw.(string); ok {
									// if it is a string, parse as JSON
									var argsMap map[string]interface{}
									if err := json.Unmarshal([]byte(argsStr), &argsMap); err == nil {
										toolCall.Function.Arguments = argsMap
									}
								} else if argsMap, ok := argsRaw.(map[string]interface{}); ok {
									// already an object, use directly
									toolCall.Function.Arguments = argsMap
								}
							}
						}

						if toolCall.ID != "" {
							msg.ToolCalls = append(msg.ToolCalls, toolCall)
						}
					}
				}
			}
		}

		// parse tool_call_id (tool role message)
		if toolCallID, ok := msgMap["tool_call_id"].(string); ok {
			msg.ToolCallID = toolCallID
		}
		if tn, ok := msgMap["tool_name"].(string); ok && strings.TrimSpace(tn) != "" {
			msg.ToolName = strings.TrimSpace(tn)
		} else if tn, ok := msgMap["name"].(string); ok && strings.TrimSpace(tn) != "" && strings.EqualFold(msg.Role, "tool") {
			msg.ToolName = strings.TrimSpace(tn)
		}

		agentMessages = append(agentMessages, msg)
	}

	// if last_react_output (assistant summary) exists, merge it as the last assistant message (consistent with save format)
	if assistantOut != "" {
		if len(agentMessages) > 0 {
			lastMsg := &agentMessages[len(agentMessages)-1]
			if strings.EqualFold(lastMsg.Role, "assistant") && len(lastMsg.ToolCalls) == 0 {
				lastMsg.Content = assistantOut
			} else {
				agentMessages = append(agentMessages, agent.ChatMessage{
					Role:    "assistant",
					Content: assistantOut,
				})
			}
		} else {
			agentMessages = append(agentMessages, agent.ChatMessage{
				Role:    "assistant",
				Content: assistantOut,
			})
		}
	}

	if len(agentMessages) == 0 {
		return nil, fmt.Errorf("messages parsed from agent trace are empty")
	}

	if h.agent != nil {
		if fixed := h.agent.RepairOrphanToolMessages(&agentMessages); fixed {
			h.logger.Info("repaired mismatched tool messages in history resumed from agent trace",
				zap.String("conversationId", conversationID),
			)
		}
	}

	h.logger.Info("resume history messages from agent trace complete",
		zap.String("conversationId", conversationID),
		zap.String("dataSource", dataSource),
		zap.Int("originalMessageCount", messageCount),
		zap.Int("finalMessageCount", len(agentMessages)),
		zap.Bool("hasAssistantOut", assistantOut != ""),
	)
	return agentMessages, nil
}

// dbMessagesToAgentChatMessages maps DB rows to agent ChatMessage for history fallback
// (includes reasoning_content for DeepSeek thinking + tool replay).
func dbMessagesToAgentChatMessages(msgs []database.Message) []agent.ChatMessage {
	out := make([]agent.ChatMessage, 0, len(msgs))
	for i := range msgs {
		m := msgs[i]
		out = append(out, agent.ChatMessage{
			Role:             m.Role,
			Content:          m.Content,
			ReasoningContent: m.ReasoningContent,
		})
	}
	return out
}
