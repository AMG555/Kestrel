package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"kestrel/internal/agentfinalizer"
	"kestrel/internal/config"
	"kestrel/internal/mcp"
	"kestrel/internal/multiagent"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// MultiAgentLoopStream is the Eino DeepAgent streaming conversation (requires config.multi_agent.enabled).
func (h *AgentHandler) MultiAgentLoopStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	if h.config == nil || !h.config.MultiAgent.Enabled {
		ev := StreamEvent{Type: "error", Message: "Multi-agent is not enabled; please enable multi_agent.enabled in settings or config.yaml"}
		b, _ := json.Marshal(ev)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		done := StreamEvent{Type: "done", Message: ""}
		db, _ := json.Marshal(done)
		fmt.Fprintf(c.Writer, "data: %s\n\n", db)
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		event := StreamEvent{Type: "error", Message: "request parameter error: " + err.Error()}
		b, _ := json.Marshal(event)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		done := StreamEvent{Type: "done", Message: ""}
		db, _ := json.Marshal(done)
		fmt.Fprintf(c.Writer, "data: %s\n\n", db)
		c.Writer.Flush()
		return
	}

	c.Header("X-Accel-Buffering", "no")

	// used in sendEvent to determine if cancellation was caused by the user actively stopping.
	// Note: baseCtx is created later; this variable is captured by the closure ahead of time.
	var baseCtx context.Context

	clientDisconnected := false
	// Shared with sseKeepalive: concurrent writes to ResponseWriter are forbidden, otherwise chunked encoding will be corrupted (ERR_INVALID_CHUNKED_ENCODING).
	var sseWriteMu sync.Mutex
	var ssePublishConversationID string
	sendEvent := func(eventType, message string, data interface{}) {
		// When user actively stops, Eino may still concurrently report eventType=="error".
		// To avoid the UI seeing two replies ("cancelled error" + "cancelled" message), discard the error corresponding to cancelled here.
		if eventType == "error" && baseCtx != nil {
			cause := context.Cause(baseCtx)
			if errors.Is(cause, ErrTaskCancelled) || errors.Is(cause, multiagent.ErrInterruptContinue) {
				return
			}
		}
		ev := StreamEvent{Type: eventType, Message: message, Data: data}
		b, errMarshal := json.Marshal(ev)
		if errMarshal != nil {
			b = []byte(`{"type":"error","message":"marshal failed"}`)
		}
		sseLine := make([]byte, 0, len(b)+8)
		sseLine = append(sseLine, []byte("data: ")...)
		sseLine = append(sseLine, b...)
		sseLine = append(sseLine, '\n', '\n')
		if ssePublishConversationID != "" && h.taskEventBus != nil {
			h.taskEventBus.Publish(ssePublishConversationID, sseLine)
		}
		if clientDisconnected {
			return
		}
		select {
		case <-c.Request.Context().Done():
			clientDisconnected = true
			return
		default:
		}
		sseWriteMu.Lock()
		_, err := c.Writer.Write(sseLine)
		if err != nil {
			sseWriteMu.Unlock()
			clientDisconnected = true
			return
		}
		if flusher, ok := c.Writer.(http.Flusher); ok {
			flusher.Flush()
		} else {
			c.Writer.Flush()
		}
		sseWriteMu.Unlock()
	}

	h.logger.Info("received Eino DeepAgent streaming request",
		zap.String("conversationId", req.ConversationID),
	)

	prep, err := h.prepareMultiAgentSession(&req, c, "multi_agent_stream")
	if err != nil {
		sendEvent("error", err.Error(), nil)
		sendEvent("done", "", nil)
		return
	}
	ssePublishConversationID = prep.ConversationID
	if prep.CreatedNew {
		sendEvent("conversation", "conversation created", map[string]interface{}{
			"conversationId": prep.ConversationID,
		})
	}

	conversationID := prep.ConversationID
	assistantMessageID := prep.AssistantMessageID
	h.activateHITLForConversation(conversationID, req.Hitl)
	if h.hitlManager != nil {
		defer h.hitlManager.DeactivateConversation(conversationID)
	}

	if prep.UserMessageID != "" {
		sendEvent("message_saved", "", map[string]interface{}{
			"conversationId": conversationID,
			"userMessageId":  prep.UserMessageID,
		})
	}
	if h.runRoleWorkflowStreamIfBound(c, &req, prep, sendEvent) {
		return
	}

	var cancelWithCause context.CancelCauseFunc
	curFinalMessage := prep.FinalMessage
	curHistory := prep.History
	roleTools := prep.RoleTools
	orch := strings.TrimSpace(req.Orchestration)

	taskStatus := "completed"
	// Only call FinishTask after StartTask succeeds; avoids incorrectly deleting a running task of the same conversation when the 'task already exists' branch returns.
	taskOwned := false
	var taskRunID string
	defer func() {
		if taskOwned {
			h.tasks.FinishTaskRun(conversationID, taskRunID, taskStatus)
		}
	}()

	sendEvent("progress", "Starting Eino multi-agent...", map[string]interface{}{
		"conversationId": conversationID,
	})

	stopKeepalive := runSSEKeepalive(c, &sseWriteMu)
	defer stopKeepalive()
	runCfg, _, err := h.configForAIChannel(req.AIChannelID)
	if err != nil {
		sendEvent("error", err.Error(), nil)
		sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
		return
	}

	var result *multiagent.RunResult
	var runErr error

	baseCtx, cancelWithCause = context.WithCancelCause(detachedAgentContext(c.Request.Context()))
	taskCtx, timeoutCancel := context.WithTimeout(baseCtx, 600*time.Minute)

	if startedTask, err := h.tasks.StartTask(conversationID, req.Message, cancelWithCause); err != nil {
		var errorMsg string
		if errors.Is(err, ErrTaskAlreadyRunning) {
			errorMsg = "⚠️ A task is already running in the current conversation. Please wait for it to complete or click \"Stop task\" before retrying."
			sendEvent("error", errorMsg, map[string]interface{}{
				"conversationId": conversationID,
				"errorType":      "task_already_running",
			})
		} else {
			errorMsg = "❌ Failed to start task: " + err.Error()
			sendEvent("error", errorMsg, nil)
		}
		if assistantMessageID != "" {
			_, _ = h.db.Exec("UPDATE messages SET content = ?, updated_at = ? WHERE id = ?", errorMsg, time.Now(), assistantMessageID)
		}
		sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
		timeoutCancel()
		return
	} else {
		taskRunID = startedTask.RunID
	}
	baseCtx = h.tasks.BindProcessScope(baseCtx, conversationID, taskRunID)
	taskCtx = h.tasks.BindProcessScope(taskCtx, conversationID, taskRunID)
	taskOwned = true
	sendEvent = h.taskFinishingEventSender(sendEvent, conversationID, taskRunID, func() string { return taskStatus })

	// Merge MCP execution IDs from multiple Run segments within the same HTTP stream (e.g. interrupt + resume), for the final response / DB table and tool chip to display the complete list
	var cumulativeMCPExecutionIDs []string
	// When running in segments within the same request, primary agent iteration events accumulate with offset to avoid the UI showing "Round 3 → Round 1" regression.
	var mainIterationOffset int
	var emptyResponseContinueAttempt int
	var finalizationAutoContinueAttempt int
	effectiveOrch := config.NormalizeMultiAgentOrchestration(h.config.MultiAgent.Orchestration)
	if o := strings.TrimSpace(req.Orchestration); o != "" {
		effectiveOrch = config.NormalizeMultiAgentOrchestration(o)
	}
	agentMode := "eino_" + effectiveOrch
	var decision agentfinalizer.Decision
	var autoCancelledPendingExecutionIDs []string

	for {
		segmentMainIterationMax := 0
		rawProgressCallback := h.createProgressCallback(taskCtx, cancelWithCause, conversationID, assistantMessageID, sendEvent)
		progressCallback := func(eventType, message string, data interface{}) {
			if eventType == "iteration" {
				if m, ok := data.(map[string]interface{}); ok {
					if scope, _ := m["einoScope"].(string); scope == "main" {
						raw := 0
						switch v := m["iteration"].(type) {
						case int:
							raw = v
						case int32:
							raw = int(v)
						case int64:
							raw = int(v)
						case float64:
							raw = int(v)
						case float32:
							raw = int(v)
						}
						if raw > 0 {
							if raw > segmentMainIterationMax {
								segmentMainIterationMax = raw
							}
							m["iteration"] = raw + mainIterationOffset
						}
					}
				}
			}
			rawProgressCallback(eventType, message, data)
		}
		taskCtxLoop := mcp.WithMCPConversationID(taskCtx, conversationID)
		taskCtxLoop = mcp.WithToolRunRegistry(taskCtxLoop, h.tasks)
		taskCtxLoop = mcp.WithEinoExecuteRunRegistry(taskCtxLoop, h.tasks)
		taskCtxLoop = multiagent.WithAgentRuntimeCancelRegistrar(taskCtxLoop, func(cancel func(error) bool) func() {
			return h.tasks.BindAgentRuntimeCancel(conversationID, cancel)
		})
		taskCtxLoop = multiagent.WithAgentTurnLoopInterruptRegistrar(taskCtxLoop, func(push func(string) bool) func() {
			return h.tasks.BindAgentTurnLoopInterrupt(conversationID, push)
		})
		taskCtxLoop = multiagent.WithHITLToolInterceptor(taskCtxLoop, func(ctx context.Context, toolName, arguments string) (string, error) {
			return h.interceptHITLForEinoTool(ctx, cancelWithCause, conversationID, assistantMessageID, sendEvent, toolName, arguments)
		})

		result, runErr = multiagent.RunDeepAgent(
			taskCtxLoop,
			runCfg,
			&runCfg.MultiAgent,
			h.agent,
			h.db,
			h.logger,
			conversationID,
			h.conversationProjectID(conversationID),
			curFinalMessage,
			curHistory,
			roleTools,
			progressCallback,
			h.agentsMarkdownDir,
			orch,
			chatReasoningToClientIntent(req.Reasoning),
			h.agentSessionContextBlock(conversationID),
		)

		if result != nil && len(result.MCPExecutionIDs) > 0 {
			cumulativeMCPExecutionIDs = mergeMCPExecutionIDLists(cumulativeMCPExecutionIDs, result.MCPExecutionIDs)
		}

		if runErr == nil {
			mw := &h.config.MultiAgent.EinoMiddleware
			if h.tryContinueOnEinoEmptyResponse(taskCtx, mw, conversationID, result, &emptyResponseContinueAttempt, &curHistory, &curFinalMessage, progressCallback) {
				mainIterationOffset += segmentMainIterationMax
				timeoutCancel()
				baseCtx, cancelWithCause, taskCtx, timeoutCancel = h.rebindEinoRunningTask(taskCtx, conversationID, timeoutCancel)
				continue
			}
			decision = h.decideAgentRunForDeliveryWithPolicy(conversationID, assistantMessageID, agentMode, result, cumulativeMCPExecutionIDs, requestRequiresExecutionEvidence(&req))
			if cancelled := h.cleanupPendingToolExecutionsAfterIteration(taskCtx, conversationID, decision, progressCallback); len(cancelled) > 0 {
				autoCancelledPendingExecutionIDs = mergeMCPExecutionIDLists(autoCancelledPendingExecutionIDs, cancelled)
				decision = h.decideAgentRunForDeliveryWithPolicy(conversationID, assistantMessageID, agentMode, result, cumulativeMCPExecutionIDs, requestRequiresExecutionEvidence(&req))
			}
			if h.tryAutoContinueAfterFinalization(taskCtx, conversationID, result, decision, &finalizationAutoContinueAttempt, &curHistory, &curFinalMessage, progressCallback) {
				mainIterationOffset += segmentMainIterationMax
				timeoutCancel()
				baseCtx, cancelWithCause, taskCtx, timeoutCancel = h.rebindEinoRunningTask(taskCtx, conversationID, timeoutCancel)
				continue
			}
			timeoutCancel()
			break
		}

		cause := context.Cause(baseCtx)
		if cause == nil {
			switch {
			case errors.Is(runErr, multiagent.ErrInterruptContinue):
				cause = multiagent.ErrInterruptContinue
			case errors.Is(runErr, ErrTaskCancelled):
				cause = ErrTaskCancelled
			}
		}
		if errors.Is(cause, multiagent.ErrInterruptContinue) {
			if shouldPersistEinoAgentTraceAfterRunError(baseCtx) {
				h.persistEinoAgentTraceForResume(conversationID, result)
			}
			note := h.tasks.TakeInterruptContinueNote(conversationID)
			icSummary := interruptContinueTimelineSummary(note)
			progressCallback("user_interrupt_continue", icSummary, map[string]interface{}{
				"conversationId": conversationID,
				"rawReason":      strings.TrimSpace(note),
				"emptyReason":    strings.TrimSpace(note) == "",
				"kind":           "no_active_mcp_tool",
			})
			inject := formatInterruptContinueUserMessage(note)
			// Not written to the messages table as a user bubble: avoids large template text in the main conversation stream; description is already recorded by user_interrupt_continue in assistant process_details (iteration details).
			if hist, err := h.loadHistoryFromAgentTrace(conversationID); err == nil && len(hist) > 0 {
				curHistory = hist
			}
			curFinalMessage = inject
			sendEvent("progress", "User supplement merged with latest trace, resuming reasoning...", map[string]interface{}{
				"conversationId": conversationID,
				"source":         "interrupt_continue",
			})
			mainIterationOffset += segmentMainIterationMax
			timeoutCancel()
			baseCtx, cancelWithCause = context.WithCancelCause(detachedAgentContext(baseCtx))
			h.tasks.BindTaskCancel(conversationID, cancelWithCause)
			taskCtx, timeoutCancel = context.WithTimeout(baseCtx, 600*time.Minute)
			h.tasks.UpdateTaskStatus(conversationID, "running")
			continue
		}

		if shouldPersistEinoAgentTraceAfterRunError(baseCtx) {
			h.persistEinoAgentTraceForResume(conversationID, result)
		}
		if errors.Is(cause, ErrTaskCancelled) {
			taskStatus = "cancelled"
			h.tasks.UpdateTaskStatus(conversationID, taskStatus)
			cancelMsg := "Task was cancelled by user; subsequent operations stopped."
			if assistantMessageID != "" {
				if result != nil {
					if err := h.mergeAssistantMessagePartialOnCancel(assistantMessageID, result.Response); err != nil {
						h.logger.Warn("merge partial reply before cancellation failed", zap.Error(err))
					}
				}
				if err := h.appendAssistantMessageNotice(assistantMessageID, cancelMsg); err != nil {
					h.logger.Warn("update assistant message after cancellation failed", zap.Error(err))
				}
				_ = h.db.AddProcessDetail(assistantMessageID, conversationID, "cancelled", cancelMsg, nil)
			}
			sendEvent("cancelled", cancelMsg, map[string]interface{}{
				"conversationId": conversationID,
				"messageId":      assistantMessageID,
			})
			sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
			timeoutCancel()
			return
		}

		if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(context.Cause(taskCtx), context.DeadlineExceeded) {
			taskStatus = "timeout"
			h.tasks.UpdateTaskStatus(conversationID, taskStatus)
			timeoutMsg := "Task execution timed out and was automatically terminated."
			if assistantMessageID != "" {
				_, _ = h.db.Exec("UPDATE messages SET content = ?, updated_at = ? WHERE id = ?", timeoutMsg, time.Now(), assistantMessageID)
				_ = h.db.AddProcessDetail(assistantMessageID, conversationID, "timeout", timeoutMsg, nil)
			}
			sendEvent("error", timeoutMsg, map[string]interface{}{
				"conversationId": conversationID,
				"messageId":      assistantMessageID,
				"errorType":      "timeout",
			})
			sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
			timeoutCancel()
			return
		}

		h.logger.Error("Eino DeepAgent execution failed", zap.Error(runErr))
		taskStatus = "failed"
		h.tasks.UpdateTaskStatus(conversationID, taskStatus)
		clientErr := multiagent.EinoClientRunErrorMessage(runErr)
		errMsg := "execution failed: " + clientErr
		if assistantMessageID != "" {
			_, _ = h.db.Exec("UPDATE messages SET content = ?, updated_at = ? WHERE id = ?", errMsg, time.Now(), assistantMessageID)
			_ = h.db.AddProcessDetail(assistantMessageID, conversationID, "error", errMsg, nil)
		}
		errData := multiagent.EinoClientRunErrorFields(runErr)
		errData["conversationId"] = conversationID
		errData["messageId"] = assistantMessageID
		errData["error"] = errMsg
		sendEvent("error", errMsg, errData)
		sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
		timeoutCancel()
		return
	}

	timeoutCancel()

	if decision.CompletionReason == "" {
		decision = h.decideAgentRunForDeliveryWithPolicy(conversationID, assistantMessageID, agentMode, result, cumulativeMCPExecutionIDs, requestRequiresExecutionEvidence(&req))
		if cancelled := h.cleanupPendingToolExecutionsAfterIteration(taskCtx, conversationID, decision, nil); len(cancelled) > 0 {
			autoCancelledPendingExecutionIDs = mergeMCPExecutionIDLists(autoCancelledPendingExecutionIDs, cancelled)
			decision = h.decideAgentRunForDeliveryWithPolicy(conversationID, assistantMessageID, agentMode, result, cumulativeMCPExecutionIDs, requestRequiresExecutionEvidence(&req))
		}
	}
	h.persistFinalizationDecision(conversationID, assistantMessageID, agentMode, cumulativeMCPExecutionIDs, multiagent.AggregatedReasoningFromTraceJSON(result.LastAgentTraceInput), decision)

	if result.LastAgentTraceInput != "" || result.LastAgentTraceOutput != "" {
		if err := h.db.SaveAgentTrace(conversationID, result.LastAgentTraceInput, result.LastAgentTraceOutput); err != nil {
			h.logger.Warn("save agent trace failed", zap.Error(err))
		}
	}

	responseText := decision.FinalText
	if !decision.Finalizable {
		responseText = finalizationBlockedMessage(decision)
		sendEvent("finalization_check", responseText, decision)
		taskStatus = decision.Status
		h.tasks.UpdateTaskStatus(conversationID, taskStatus)
	}
	sendEvent("response", responseText, finalizationResponsePayload(decision, map[string]interface{}{
		"mcpExecutionIds":                  cumulativeMCPExecutionIDs,
		"conversationId":                   conversationID,
		"messageId":                        assistantMessageID,
		"agentMode":                        agentMode,
		"autoCancelledPendingExecutionIds": autoCancelledPendingExecutionIDs,
	}))
	sendEvent("done", "", map[string]interface{}{"conversationId": conversationID})
}

// MultiAgentLoop is the Eino DeepAgent non-streaming conversation (requires multi_agent.enabled).
func (h *AgentHandler) MultiAgentLoop(c *gin.Context) {
	if h.config == nil || !h.config.MultiAgent.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "Multi-agent is not enabled; please set multi_agent.enabled: true in config.yaml"})
		return
	}

	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.logger.Info("received Eino DeepAgent non-streaming request", zap.String("conversationId", req.ConversationID))

	prep, err := h.prepareMultiAgentSession(&req, c, "multi_agent")
	if err != nil {
		status, msg := multiAgentHTTPErrorStatus(err)
		c.JSON(status, gin.H{"error": msg})
		return
	}
	h.activateHITLForConversation(prep.ConversationID, req.Hitl)
	if h.hitlManager != nil {
		defer h.hitlManager.DeactivateConversation(prep.ConversationID)
	}
	if h.runRoleWorkflowJSONIfBound(c, &req, prep) {
		return
	}

	baseCtx, cancelWithCause := context.WithCancelCause(c.Request.Context())
	defer cancelWithCause(nil)
	taskCtx, timeoutCancel := context.WithTimeout(baseCtx, 600*time.Minute)
	defer timeoutCancel()
	jsonTask, startErr := h.tasks.StartTask(prep.ConversationID, req.Message, cancelWithCause)
	if startErr != nil {
		c.JSON(http.StatusConflict, gin.H{"error": startErr.Error()})
		return
	}
	taskCtx = h.tasks.BindProcessScope(taskCtx, prep.ConversationID, jsonTask.RunID)
	taskCtx = mcp.WithMCPConversationID(taskCtx, prep.ConversationID)
	taskCtx = mcp.WithToolRunRegistry(taskCtx, h.tasks)
	taskCtx = mcp.WithEinoExecuteRunRegistry(taskCtx, h.tasks)
	jsonTaskStatus := "failed"
	defer func() { _ = h.tasks.FinishTaskRun(prep.ConversationID, jsonTask.RunID, jsonTaskStatus) }()
	respond := h.taskFinishingJSONResponder(c, prep.ConversationID, jsonTask.RunID, func() string { return jsonTaskStatus })

	progressCallback := h.createProgressCallback(taskCtx, cancelWithCause, prep.ConversationID, prep.AssistantMessageID, nil)
	taskCtx = multiagent.WithHITLToolInterceptor(taskCtx, func(ctx context.Context, toolName, arguments string) (string, error) {
		return h.interceptHITLForEinoTool(ctx, cancelWithCause, prep.ConversationID, prep.AssistantMessageID, nil, toolName, arguments)
	})
	runCfg, _, err := h.configForAIChannel(req.AIChannelID)
	if err != nil {
		respond(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	curHist := prep.History
	curMsg := prep.FinalMessage
	var result *multiagent.RunResult
	var runErr error
	var emptyResponseContinueAttempt int
	var finalizationAutoContinueAttempt int
	effectiveOrch := config.NormalizeMultiAgentOrchestration(h.config.MultiAgent.Orchestration)
	if o := strings.TrimSpace(req.Orchestration); o != "" {
		effectiveOrch = config.NormalizeMultiAgentOrchestration(o)
	}
	agentMode := "eino_" + effectiveOrch
	var decision agentfinalizer.Decision
	var autoCancelledPendingExecutionIDs []string
	for {
		result, runErr = multiagent.RunDeepAgent(
			taskCtx,
			runCfg,
			&runCfg.MultiAgent,
			h.agent,
			h.db,
			h.logger,
			prep.ConversationID,
			h.conversationProjectID(prep.ConversationID),
			curMsg,
			curHist,
			prep.RoleTools,
			progressCallback,
			h.agentsMarkdownDir,
			strings.TrimSpace(req.Orchestration),
			chatReasoningToClientIntent(req.Reasoning),
			h.agentSessionContextBlock(prep.ConversationID),
		)
		if runErr != nil {
			if shouldPersistEinoAgentTraceAfterRunError(baseCtx) {
				h.persistEinoAgentTraceForResume(prep.ConversationID, result)
			}
			h.logger.Error("Eino DeepAgent execution failed", zap.Error(runErr))
			clientErr := multiagent.EinoClientRunErrorMessage(runErr)
			errMsg := "execution failed: " + clientErr
			if prep.AssistantMessageID != "" {
				_, _ = h.db.Exec("UPDATE messages SET content = ?, updated_at = ? WHERE id = ?", errMsg, time.Now(), prep.AssistantMessageID)
			}
			errData := multiagent.EinoClientRunErrorFields(runErr)
			errData["error"] = errMsg
			respond(http.StatusInternalServerError, errData)
			return
		}
		mw := &h.config.MultiAgent.EinoMiddleware
		if h.tryContinueOnEinoEmptyResponse(taskCtx, mw, prep.ConversationID, result, &emptyResponseContinueAttempt, &curHist, &curMsg, progressCallback) {
			continue
		}
		decision = h.decideAgentRunForDeliveryWithPolicy(prep.ConversationID, prep.AssistantMessageID, agentMode, result, result.MCPExecutionIDs, requestRequiresExecutionEvidence(&req))
		if cancelled := h.cleanupPendingToolExecutionsAfterIteration(taskCtx, prep.ConversationID, decision, progressCallback); len(cancelled) > 0 {
			autoCancelledPendingExecutionIDs = mergeMCPExecutionIDLists(autoCancelledPendingExecutionIDs, cancelled)
			decision = h.decideAgentRunForDeliveryWithPolicy(prep.ConversationID, prep.AssistantMessageID, agentMode, result, result.MCPExecutionIDs, requestRequiresExecutionEvidence(&req))
		}
		if h.tryAutoContinueAfterFinalization(taskCtx, prep.ConversationID, result, decision, &finalizationAutoContinueAttempt, &curHist, &curMsg, progressCallback) {
			continue
		}
		break
	}

	h.persistFinalizationDecision(prep.ConversationID, prep.AssistantMessageID, agentMode, result.MCPExecutionIDs, multiagent.AggregatedReasoningFromTraceJSON(result.LastAgentTraceInput), decision)

	if result.LastAgentTraceInput != "" || result.LastAgentTraceOutput != "" {
		if err := h.db.SaveAgentTrace(prep.ConversationID, result.LastAgentTraceInput, result.LastAgentTraceOutput); err != nil {
			h.logger.Warn("save agent trace failed", zap.Error(err))
		}
	}

	responseText := decision.FinalText
	if !decision.Finalizable {
		responseText = finalizationBlockedMessage(decision)
	}

	jsonTaskStatus = decision.Status
	if jsonTaskStatus == "" {
		jsonTaskStatus = "completed"
	}
	respond(http.StatusOK, ChatResponse{
		Response:                         responseText,
		MCPExecutionIDs:                  result.MCPExecutionIDs,
		ConversationID:                   prep.ConversationID,
		Time:                             time.Now(),
		Finalizable:                      decision.Finalizable,
		Finalized:                        decision.Finalized,
		Status:                           decision.Status,
		CompletionReason:                 decision.CompletionReason,
		EvidenceVerified:                 decision.EvidenceVerified,
		EvidenceRefs:                     decision.EvidenceRefs,
		PendingExecutionIDs:              decision.PendingExecutionIDs,
		MissingChecks:                    decision.MissingChecks,
		AutoCancelledPendingExecutionIDs: autoCancelledPendingExecutionIDs,
	})
}

// persistEinoAgentTraceForResume writes the agent trace (DB columns last_react_*) when Eino exits abnormally, for the next request's loadHistoryFromAgentTrace soft-resume.
func (h *AgentHandler) persistEinoAgentTraceForResume(conversationID string, result *multiagent.RunResult) {
	if h == nil || result == nil {
		return
	}
	if result.LastAgentTraceInput == "" && result.LastAgentTraceOutput == "" {
		return
	}
	if err := h.db.SaveAgentTrace(conversationID, result.LastAgentTraceInput, result.LastAgentTraceOutput); err != nil {
		h.logger.Warn("save Eino resume context failed", zap.String("conversationId", conversationID), zap.Error(err))
	}
}

// mergeMCPExecutionIDLists deduplicates and merges MCP execution IDs from multiple Run segments (order: dst first, then more).
func mergeMCPExecutionIDLists(dst []string, more []string) []string {
	seen := make(map[string]struct{}, len(dst)+len(more))
	out := make([]string, 0, len(dst)+len(more))
	add := func(ids []string) {
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	add(dst)
	add(more)
	return out
}

// interruptContinueTimelineSummary is the short body displayed in timeline / process_details (full template is written in another user message).
func interruptContinueTimelineSummary(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return "User chose to interrupt and continue without providing notes; context merged using default pentest supplement template and resumed."
	}
	return "User interrupt note (original text):\n\n" + note
}

// formatInterruptContinueUserMessage formats the note from the 'interrupt and continue' dialog into a new user message (emphasizes path supplement and port rescan in pentest scenarios).
func formatInterruptContinueUserMessage(note string) string {
	var b strings.Builder
	b.WriteString("[User Supplement / Post-interrupt Continue]\n")
	if s := strings.TrimSpace(note); s != "" {
		b.WriteString(s)
		b.WriteString("\n\n")
	}
	b.WriteString("[Action items for this round]\n")
	b.WriteString("- Incorporate the interface paths, parameters, and business changes provided by the user into subsequent testing and reasoning.\n")
	b.WriteString("- If asset or target information has been updated, re-run port/service scanning on the target, then plan the next step based on the new results.\n")
	b.WriteString("- Build on the existing trace; avoid meaninglessly repeating already-completed steps.\n")
	return strings.TrimSpace(b.String())
}

func multiAgentHTTPErrorStatus(err error) (int, string) {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "access denied"):
		return http.StatusForbidden, msg
	case strings.Contains(msg, "conversation not found"):
		return http.StatusNotFound, msg
	case strings.Contains(msg, "WebShell not found"):
		return http.StatusBadRequest, msg
	case strings.Contains(msg, "too many attachments"), strings.Contains(msg, "attachments"):
		return http.StatusBadRequest, msg
	case strings.Contains(msg, "saveuser messagefailed"), strings.Contains(msg, "create conversationfailed"):
		return http.StatusInternalServerError, msg
	case strings.Contains(msg, "saveupload filefailed"):
		return http.StatusInternalServerError, msg
	default:
		return http.StatusBadRequest, msg
	}
}
