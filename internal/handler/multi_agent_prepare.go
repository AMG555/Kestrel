package handler

import (
	"fmt"
	"strings"

	"kestrel/internal/agent"
	"kestrel/internal/audit"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/mcp/builtin"
	"kestrel/internal/security"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// multiAgentPrepared holds the conversation and message preparation result for a multi-agent request before calling Eino.
type multiAgentPrepared struct {
	ConversationID     string
	CreatedNew         bool
	History            []agent.ChatMessage
	FinalMessage       string
	RoleTools          []string
	AssistantMessageID string
	UserMessageID      string
}

func chatRequestAgentMode(req *ChatRequest, source string) string {
	if strings.HasPrefix(strings.TrimSpace(source), "multi_agent") {
		return config.NormalizeMultiAgentOrchestration(req.Orchestration)
	}
	return "eino_single"
}

func (h *AgentHandler) prepareMultiAgentSession(req *ChatRequest, c *gin.Context, source string) (*multiAgentPrepared, error) {
	if len(req.Attachments) > maxAttachments {
		return nil, fmt.Errorf("at most %d attachments allowed", maxAttachments)
	}

	conversationID := strings.TrimSpace(req.ConversationID)
	projectID := strings.TrimSpace(effectiveProjectID(h.config, req.ProjectID))
	webshellID := strings.TrimSpace(req.WebShellConnectionID)
	session, hasSession := security.CurrentSession(c)
	if !hasSession || !session.Permissions["chat:write"] {
		return nil, fmt.Errorf("no permission to write to conversation")
	}
	canAccess := func(resourceType, resourceID string) bool {
		if !hasSession || h.db == nil || strings.TrimSpace(resourceID) == "" {
			return false
		}
		return h.db.UserCanAccessResource(session.UserID, session.Scope, resourceType, resourceID)
	}
	if projectID != "" && (!session.Permissions["project:read"] || !canAccess("project", projectID)) {
		return nil, fmt.Errorf("access deniedtarget project")
	}
	if webshellID != "" && (!session.Permissions["webshell:write"] || !canAccess("webshell", webshellID)) {
		return nil, fmt.Errorf("access denied for this WebShell connection")
	}
	createdNew := false
	if conversationID == "" {
		title := safeTruncateString(req.Message, 50)
		var conv *database.Conversation
		var err error
		meta := audit.ConversationCreateMetaFromGin(c, source)
		meta.ProjectID = projectID
		meta.RoleName = req.Role
		meta.AgentMode = chatRequestAgentMode(req, source)
		if webshellID != "" {
			meta.Source = source + "_webshell"
			meta.WebShellConnectionID = webshellID
			conv, err = h.db.CreateConversationWithWebshell(meta.WebShellConnectionID, title, meta)
		} else {
			conv, err = h.db.CreateConversation(title, meta)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to create conversation: %w", err)
		}
		conversationID = conv.ID
		createdNew = true
		if hasSession {
			_ = h.db.SetResourceOwner("conversation", conversationID, session.UserID)
			_ = h.db.AssignResourceToUser(session.UserID, "conversation", conversationID)
		}
	} else {
		if _, err := h.db.GetConversation(conversationID); err != nil {
			return nil, fmt.Errorf("conversation not found")
		}
		if !canAccess("conversation", conversationID) {
			return nil, fmt.Errorf("access denied for this conversation")
		}
	}
	if err := h.db.SetConversationRoleName(conversationID, req.Role); err != nil {
		h.logger.Warn("failed to update conversation role", zap.String("conversationId", conversationID), zap.String("role", req.Role), zap.Error(err))
	}
	if err := h.db.SetConversationAgentMode(conversationID, chatRequestAgentMode(req, source)); err != nil {
		h.logger.Warn("updateconversationpatternfailed", zap.String("conversationId", conversationID), zap.String("source", source), zap.String("orchestration", req.Orchestration), zap.Error(err))
	}

	agentHistoryMessages, err := h.loadHistoryFromAgentTrace(conversationID)
	if err != nil {
		historyMessages, getErr := h.db.GetMessages(conversationID)
		if getErr != nil {
			agentHistoryMessages = []agent.ChatMessage{}
		} else {
			agentHistoryMessages = dbMessagesToAgentChatMessages(historyMessages)
		}
	}

	finalMessage := req.Message
	var roleTools []string
	if webshellID != "" {
		conn, errConn := h.db.GetWebshellConnection(webshellID)
		if errConn != nil || conn == nil {
			h.logger.Warn("WebShell AI assistant: connection not found", zap.String("id", req.WebShellConnectionID), zap.Error(errConn))
			return nil, fmt.Errorf("WebShell connection not found")
		}
		webshellContext := BuildWebshellAssistantContext(conn, WebshellSkillHintMultiAgent, req.Message)
		// In WebShell mode, if a role is also specified, append the role user_prompt (tool set is still limited to webshell-specific tools).
		if req.Role != "" && req.Role != "default" && h.config != nil && h.config.Roles != nil {
			if role, exists := h.config.Roles[req.Role]; exists && role.Enabled && role.UserPrompt != "" {
				finalMessage = role.UserPrompt + "\n\n" + webshellContext
				h.logger.Info("WebShell + role: applying role prompt (multi-agent)", zap.String("role", req.Role))
			} else {
				finalMessage = webshellContext
			}
		} else {
			finalMessage = webshellContext
		}
		roleTools = []string{
			builtin.ToolWebshellExec,
			builtin.ToolWebshellFileList,
			builtin.ToolWebshellFileRead,
			builtin.ToolWebshellFileWrite,
			builtin.ToolRecordVulnerability,
			builtin.ToolListVulnerabilities,
			builtin.ToolGetVulnerability,
			builtin.ToolUpsertProjectFact,
			builtin.ToolGetProjectFact,
			builtin.ToolListProjectFacts,
			builtin.ToolSearchProjectFacts,
			builtin.ToolDeprecateProjectFact,
			builtin.ToolRestoreProjectFact,
			builtin.ToolListKnowledgeRiskTypes,
			builtin.ToolSearchKnowledgeBase,
		}
	} else if req.Role != "" && req.Role != "default" && h.config != nil && h.config.Roles != nil {
		if role, exists := h.config.Roles[req.Role]; exists && role.Enabled {
			if role.UserPrompt != "" {
				finalMessage = role.UserPrompt + "\n\n" + req.Message
			}
			roleTools = role.Tools
		}
	}

	var savedPaths []string
	if len(req.Attachments) > 0 {
		var aerr error
		savedPaths, aerr = saveAttachmentsToDateAndConversationDir(req.Attachments, conversationID, h.logger)
		if aerr != nil {
			return nil, fmt.Errorf("saveupload filefailed: %w", aerr)
		}
	}
	finalMessage = appendAttachmentsToMessage(finalMessage, req.Attachments, savedPaths)

	userContent := userMessageContentForStorage(req.Message, req.Attachments, savedPaths)
	userMsgRow, uerr := h.db.AddMessage(conversationID, "user", userContent, nil)
	if uerr != nil {
		h.logger.Error("saveuser messagefailed", zap.Error(uerr))
		return nil, fmt.Errorf("failed to save user message: %w", uerr)
	}
	userMessageID := ""
	if userMsgRow != nil {
		userMessageID = userMsgRow.ID
	}

	assistantMsg, aerr := h.db.AddMessage(conversationID, "assistant", "processing...", nil)
	var assistantMessageID string
	if aerr != nil {
		h.logger.Warn("failed to create assistant message placeholder", zap.Error(aerr))
	} else if assistantMsg != nil {
		assistantMessageID = assistantMsg.ID
	}

	return &multiAgentPrepared{
		ConversationID:     conversationID,
		CreatedNew:         createdNew,
		History:            agentHistoryMessages,
		FinalMessage:       finalMessage,
		RoleTools:          roleTools,
		AssistantMessageID: assistantMessageID,
		UserMessageID:      userMessageID,
	}, nil
}
