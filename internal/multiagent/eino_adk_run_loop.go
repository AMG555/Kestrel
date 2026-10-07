package multiagent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kestrel/internal/agent"
	"kestrel/internal/config"
	"kestrel/internal/einomcp"
	"kestrel/internal/einoobserve"
	"kestrel/internal/security"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

// normalizeStreamingDelta normalizes a chunk that may be an "accumulated fragment" into a "pure delta".
// Some models/bridge layers repeatedly send already-output prefixes during streaming; if the frontend does buffer+=chunk directly, it will show duplicate text.
//
// Note: keep consistent with internal/openai.normalizeStreamingDelta.
func normalizeStreamingDelta(current, incoming string) (next, delta string) {
	if incoming == "" {
		return current, ""
	}
	if current == "" {
		return incoming, incoming
	}
	if strings.HasPrefix(incoming, current) && len(incoming) > len(current) {
		return incoming, incoming[len(current):]
	}
	if incoming == current && utf8.RuneCountInString(current) > 1 {
		return current, ""
	}
	return current + incoming, incoming
}

func isInterruptContinue(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return errors.Is(context.Cause(ctx), ErrInterruptContinue)
}

func isEinoStreamCanceled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, adk.ErrStreamCanceled) {
		return true
	}
	var streamCanceled *adk.StreamCanceledError
	return errors.As(err, &streamCanceled)
}

func isEinoCancelError(err error) bool {
	if err == nil {
		return false
	}
	var cancelErr *adk.CancelError
	return errors.As(err, &cancelErr)
}

// isEinoVoluntaryCancelErr reports cancel signals produced by Agent Cancel /
// TurnLoop preempt (CancelError, ErrStreamCanceled, context.Canceled).
func isEinoVoluntaryCancelErr(err error) bool {
	if err == nil {
		return false
	}
	return isEinoCancelError(err) || isEinoStreamCanceled(err) || errors.Is(err, context.Canceled)
}

// isEinoTurnLoopPreemptErr is true when a cancel/stream-cancel leaked from the
// current agent turn while the host task context is still alive. TurnLoop
// interrupt-continue does not cancel the parent context; treating that leak as
// fatal would abort the whole run instead of starting the queued next turn.
func isEinoTurnLoopPreemptErr(ctx context.Context, err error) bool {
	if err == nil || !isEinoVoluntaryCancelErr(err) {
		return false
	}
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	return true
}

func isEinoIterationLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "max iteration") ||
		strings.Contains(msg, "maximum iteration") ||
		strings.Contains(msg, "maximum iterations") ||
		strings.Contains(msg, "iteration limit") ||
		strings.Contains(msg, "reached maximum iterations")
}

// einoADKRunLoopArgs extracts the Eino adk.Runner event loop from RunDeepAgent / RunEinoSingleChatModelAgent for reuse.
type einoADKRunLoopArgs struct {
	OrchMode             string
	OrchestratorName     string
	ConversationID       string
	Progress             func(eventType, message string, data interface{})
	Logger               *zap.Logger
	SnapshotMCPIDs       func() []string
	StreamsMainAssistant func(agent string) bool
	EinoRoleTag          func(agent string) string
	CheckpointDir        string
	// RunRetryMaxAttempts / RunRetryMaxBackoffSec: exponential backoff resume on 429, 5xx, network jitter (0 = default 4 attempts / 30s ceiling).
	RunRetryMaxAttempts   int
	RunRetryMaxBackoffSec int

	McpIDsMu *sync.Mutex
	McpIDs   *[]string

	// FilesystemMonitorAgent / FilesystemMonitorRecord: when non-nil, records Eino ADK filesystem middleware tools (ls/read_file/write_file/edit_file/glob/grep）
	// to MCP monitor on completion; execute is still recorded by eino_execute_monitor and skipped here.
	FilesystemMonitorAgent  *agent.Agent
	FilesystemMonitorRecord einomcp.ExecutionRecorder
	MCPExecutionBinder      *MCPExecutionBinder

	// ToolInvokeNotify is shared with einomcp.ToolsFromDefinitions: the run loop Sets before each iteration; execute/MCP bridge Fires to immediately push tool_result (ADK late-arrival via toolResultEmitter deduplicated).
	ToolInvokeNotify *einomcp.ToolInvokeNotifyHolder

	DA adk.Agent

	// EmptyResponseMessage is the placeholder when no assistant body text is captured (multi-agent and single-agent have different text).
	EmptyResponseMessage string

	// ModelFacingTrace is optional: written by the last middleware in each ChatModelAgent Handlers chain as a snapshot of messages about to be sent to the model;
	// when non-nil, it takes priority for LastAgentTraceInput serialization so that resume is consistent with the context after summarization/reduction.
	ModelFacingTrace *modelFacingTraceHolder

	// EinoCallbacks is optional: injects eino [callbacks] full-chain observability into the ADK Runner (see internal/einoobserve).
	EinoCallbacks *config.MultiAgentEinoCallbacksConfig

	// MaxTotalTokens / ToolMaxBytes / ModelName are used for aggressive compression resume on context overflow.
	MaxTotalTokens   int
	ToolMaxBytes     int
	ModelName        string
	MiddlewareConfig *config.MultiAgentEinoMiddlewareConfig

	// TurnLoopInterruptTimeout is for test / special runtime override only; 0 uses the EinoTurnLoopRuntime default value.
	TurnLoopInterruptTimeout time.Duration
}

func runEinoADKAgentLoop(ctx context.Context, args *einoADKRunLoopArgs, baseMsgs []adk.Message) (*RunResult, error) {
	if args == nil || args.DA == nil {
		return nil, fmt.Errorf("eino run loop: args or Agent is nil")
	}
	if args.McpIDs == nil {
		s := []string{}
		args.McpIDs = &s
	}
	if args.McpIDsMu == nil {
		args.McpIDsMu = &sync.Mutex{}
	}

	orchMode := args.OrchMode
	orchestratorName := args.OrchestratorName
	conversationID := args.ConversationID
	progress := args.Progress
	logger := args.Logger
	runID := newEinoRunID()
	progress = withEinoRunIDProgress(runID, progress)
	args.Progress = progress
	if logger != nil {
		logger.Info("eino run session started",
			zap.String("runId", runID),
			zap.String("conversationId", conversationID),
			zap.String("orchestration", orchMode),
			zap.String("orchestratorName", orchestratorName),
		)
	}
	snapshotMCPIDs := args.SnapshotMCPIDs
	if snapshotMCPIDs == nil {
		snapshotMCPIDs = func() []string { return nil }
	}
	streamsMainAssistant := args.StreamsMainAssistant
	if streamsMainAssistant == nil {
		streamsMainAssistant = func(agent string) bool {
			return agent == "" || agent == orchestratorName
		}
	}
	einoRoleTag := args.EinoRoleTag
	if einoRoleTag == nil {
		einoRoleTag = func(agent string) string {
			if streamsMainAssistant(agent) {
				return "orchestrator"
			}
			return "sub"
		}
	}
	// panic recovery: prevents internal Eino framework panics from crashing the entire goroutine and leaving connections unable to close normally.
	defer func() {
		if r := recover(); r != nil {
			if logger != nil {
				logger.Error("eino runner panic recovered", zap.Any("recover", r), zap.Stack("stack"))
			}
			if progress != nil {
				progress("error", fmt.Sprintf("Internal error: %v / internal error: %v", r, r), map[string]interface{}{
					"conversationId": conversationID,
					"source":         "eino",
				})
			}
		}
	}()

	msgs := append([]adk.Message(nil), baseMsgs...)

	emptyHint := strings.TrimSpace(args.EmptyResponseMessage)
	if emptyHint == "" {
		emptyHint = "(Eino session completed but no assistant text was captured. Check process details or logs.) " +
			"(Eino session completed, but no assistant text output was captured. Please check process details or logs.)"
	}

	if args.EinoCallbacks != nil {
		ctx = einoobserve.AttachAgentRunCallbacks(ctx, args.EinoCallbacks, einoobserve.Params{
			Logger:           logger,
			Progress:         progress,
			ConversationID:   conversationID,
			OrchMode:         orchMode,
			OrchestratorName: orchestratorName,
			RunID:            runID,
		})
	}

	drain := newEinoRunEventDrain(einoRunEventDrainConfig{
		Context:                 ctx,
		ConversationID:          conversationID,
		OrchMode:                orchMode,
		OrchestratorName:        orchestratorName,
		Progress:                progress,
		Logger:                  logger,
		BaseMessages:            msgs,
		SnapshotMCPIDs:          snapshotMCPIDs,
		StreamsMainAssistant:    streamsMainAssistant,
		EinoRoleTag:             einoRoleTag,
		MiddlewareConfig:        args.MiddlewareConfig,
		FilesystemMonitorAgent:  args.FilesystemMonitorAgent,
		FilesystemMonitorRecord: args.FilesystemMonitorRecord,
		MCPExecutionBinder:      args.MCPExecutionBinder,
	})
	session := newEinoRunRuntimeSession(einoRunRuntimeSessionConfig{
		Context:        ctx,
		Args:           args,
		Drain:          drain,
		BaseMessages:   msgs,
		EmptyHint:      emptyHint,
		SnapshotMCPIDs: snapshotMCPIDs,
		EinoRoleTag:    einoRoleTag,
	})
	defer session.Close()

	// Only reset after actually receiving data / completing a step after backoff retry, to avoid the first non-error ADK event after restart incorrectly resetting the count to 0.
	drain.BindHandlers(session.ConfirmRecovery)

	for {
		// iter.Next may block for a long time (tool execution, model inference); must be linked with ctx, otherwise cancellation/timeout cannot flush pending items promptly.
		ev, ok, iterCtxErr := nextAgentEventWithContext(ctx, session.Iterator())
		if iterCtxErr != nil {
			return session.HandleIteratorContextError(iterCtxErr)
		}
		if !ok {
			// iter ending does not always mean "normal completion":
			// when cancellation/timeout occurs while iter.Next() is blocking, it may return !ok directly.
			// checkpoint must be preserved here to avoid being misidentified as "no breakpoint" and triggering a full rerun on resume.
			completed, result, err := session.HandleIteratorEnd()
			if result != nil || err != nil {
				return result, err
			}
			if completed {
				break
			}
			continue
		}
		if ev == nil {
			continue
		}
		if ev.Err != nil {
			handled := session.HandleRunError(ev.Err)
			if handled.Result != nil || handled.Err != nil {
				return handled.Result, handled.Err
			}
			if handled.Restarted {
				continue
			}
		}
		drain.ObserveAgent(ev.AgentName)
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mv := ev.Output.MessageOutput

		if drain.HandleToolResultStreaming(mv, ev.AgentName) {
			continue
		}

		if handledStream, streamRecvErr := drain.HandleAssistantStream(mv, ev.AgentName); handledStream {
			if streamRecvErr != nil {
				handled := session.HandleStreamError(streamRecvErr, ev.AgentName)
				if handled.Result != nil || handled.Err != nil {
					return handled.Result, handled.Err
				}
				if handled.Restarted {
					continue
				}
			} else {
				session.ConfirmRecovery()
			}
			continue
		}

		msg, gerr := mv.GetMessage()
		if gerr != nil || msg == nil {
			continue
		}
		drain.HandleMaterialized(mv, msg, ev.AgentName)
		session.ConfirmRecovery()
	}

	return session.BuildFinalResult(), nil
}

// modelFacingTraceSnapshot returns only the state that actually reached the model boundary.
// Never fall back to event-stream accumulation here: it can contain pre-reduction tool output
// that the model never received (for example when summarization failed before the first call).
func modelFacingTraceSnapshot(args *einoADKRunLoopArgs) []adk.Message {
	if args != nil && args.ModelFacingTrace != nil {
		if snap := args.ModelFacingTrace.Snapshot(); len(snap) > 0 {
			return snap
		}
	}
	return nil
}

// friendlyEinoExecuteInvokeTail converts Eino execute timeout/interruption/stream abnormality into a short hint.
// Non-zero exit commands (ExecuteExitError) already have aligned exec body text; do not append "execution did not end normally".
func friendlyEinoExecuteInvokeTail(invokeErr error) string {
	if invokeErr == nil {
		return ""
	}
	var exitErr *ExecuteExitError
	if errors.As(invokeErr, &exitErr) {
		return ""
	}
	if errors.Is(invokeErr, context.DeadlineExceeded) {
		return einoExecuteTimeoutUserHint()
	}
	if errors.Is(invokeErr, context.Canceled) {
		return ""
	}
	if strings.Contains(invokeErr.Error(), "shell inactivity timeout") {
		return ""
	}
	return "[Execution did not end normally] " + invokeErr.Error()
}

// einoToolResultIsError uniformly determines whether an Eino tool result should be marked as error (aligned with MCP exec's IsError).
func einoToolResultIsError(toolName, content string) bool {
	if strings.HasPrefix(content, einomcp.ToolErrorPrefix) {
		return true
	}
	if strings.TrimSpace(toolName) == "execute" && security.IsCommandFailureResult(content) {
		return true
	}
	return false
}

func isMCPBackgroundWaitResult(content string) bool {
	text := strings.ToLower(strings.TrimSpace(content))
	if text == "" {
		return false
	}
	hasExecutionID := strings.Contains(text, "execution_id:") || strings.Contains(text, `"execution_id"`)
	hasRunningStatus := strings.Contains(text, "status: running") || strings.Contains(text, "status: queued") ||
		strings.Contains(text, `"status": "running"`) || strings.Contains(text, `"status":"running"`) ||
		strings.Contains(text, `"status": "queued"`) || strings.Contains(text, `"status":"queued"`)
	hasSoftWaitSignal := strings.Contains(text, "Tool submitted to background execution") ||
		strings.Contains(text, "this wait has reached the limit") ||
		strings.Contains(text, "wait_timeout:") ||
		strings.Contains(text, "background execution") ||
		strings.Contains(text, "still running") ||
		strings.Contains(text, "still not completed")
	return hasExecutionID && hasRunningStatus && hasSoftWaitSignal
}

func mcpExecutionIDFromWaitResult(content string) string {
	re := regexp.MustCompile(`(?i)"?execution_id"?\s*[:=]\s*"?([0-9a-f]{8}-[0-9a-f-]{12,})"?`)
	if m := re.FindStringSubmatch(content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if !strings.HasPrefix(lower, "execution_id:") {
			continue
		}
		return strings.Trim(strings.TrimSpace(line[len("execution_id:"):]), `"'`)
	}
	return ""
}

// einoToolResultBody removes the tool error prefix and returns the display/persistence body text.
func einoToolResultBody(content string) string {
	if strings.HasPrefix(content, einomcp.ToolErrorPrefix) {
		return strings.TrimPrefix(content, einomcp.ToolErrorPrefix)
	}
	return content
}

// nextAgentEventWithContext stops blocking indefinitely on iter.Next() when ctx is cancelled (common during tool execution/model inference).
func nextAgentEventWithContext(ctx context.Context, iter *adk.AsyncIterator[*adk.AgentEvent]) (ev *adk.AgentEvent, ok bool, ctxErr error) {
	if iter == nil {
		return nil, false, nil
	}
	type nextRes struct {
		ev *adk.AgentEvent
		ok bool
	}
	ch := make(chan nextRes, 1)
	go func() {
		e, o := iter.Next()
		ch <- nextRes{e, o}
	}()
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case res := <-ch:
		return res.ev, res.ok, nil
	}
}

// recvSchemaMessageStream consumes ADK Tool streaming results; returns immediately when ctx is cancelled, to avoid permanent blocking when tools like amass produce no output.
func recvSchemaMessageStream(ctx context.Context, stream *schema.StreamReader[*schema.Message]) (content, toolCallID, toolName string, recvErr error) {
	msgs, recvErr := recvSchemaToolResultMessages(ctx, stream)
	if len(msgs) == 0 {
		return "", "", "", recvErr
	}
	parts := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		parts = append(parts, msg.Content)
		if id := strings.TrimSpace(msg.ToolCallID); id != "" {
			toolCallID = id
		}
		if name := strings.TrimSpace(msg.ToolName); name != "" {
			toolName = name
		}
	}
	return strings.Join(parts, ""), toolCallID, toolName, recvErr
}

// recvSchemaToolResultMessages first collects the full Tool stream, then merges using Eino ConcatMessages.
// EventSender: when one call has one stream, uses ConcatMessages; when parallel results are flattened into the same stream, splits by CallID then merges.
func recvSchemaToolResultMessages(ctx context.Context, stream *schema.StreamReader[*schema.Message]) (msgs []*schema.Message, recvErr error) {
	if stream == nil {
		return nil, nil
	}
	var chunks []*schema.Message
	recvErr = recvEinoSchemaMessageStreamWithContext(ctx, stream, 8, func(chunk *schema.Message) {
		chunks = append(chunks, chunk)
	})
	msgs, concatErr := concatToolResultChunks(chunks)
	if concatErr != nil && recvErr == nil {
		return nil, concatErr
	}
	return msgs, recvErr
}

func buildEinoCheckpointID(orchMode string) string {
	mode := sanitizeEinoPathSegment(strings.TrimSpace(orchMode))
	if mode == "" {
		mode = "default"
	}
	return "runner-" + mode
}

func buildEinoTurnLoopCheckpointID(orchMode string) string {
	mode := sanitizeEinoPathSegment(strings.TrimSpace(orchMode))
	if mode == "" {
		mode = "default"
	}
	return "turn-loop-" + mode
}
