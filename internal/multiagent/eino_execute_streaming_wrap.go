package multiagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"kestrel/internal/einomcp"
	"kestrel/internal/mcp"
	"kestrel/internal/security"

	"github.com/cloudwego/eino/adk/filesystem"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// prependPythonUnbufferedEnv injects PYTHONUNBUFFERED=1 for /bin/sh -c.
// eino-ext local pushes streaming stdout via bufio line-by-line; python3 uses block-buffering when writing to a pipe by default, leaving print in user-space buffers for a long time,
// so the pipe receives no newlines, appearing as long periods of no output until timeout or exit. If PYTHONUNBUFFERED already appears in the command, do not override.
func prependPythonUnbufferedEnv(shellCommand string) string {
	if strings.TrimSpace(shellCommand) == "" {
		return shellCommand
	}
	if strings.Contains(strings.ToUpper(shellCommand), "PYTHONUNBUFFERED") {
		return shellCommand
	}
	return "export PYTHONUNBUFFERED=1\n" + shellCommand
}

// einoExecuteTimeoutUserHint is consistent with what is written to the ADK tool message (model-visible) and the SSE tool_result tail marker.
func einoExecuteTimeoutUserHint() string {
	return "Timed out and terminated · Timed out"
}

// einoExecuteRecvErrIsToolTimeout determines whether a Recv error was triggered by agent.tool_timeout_minutes.
// After WithTimeout expires, the local side often reports canceled / exit -1, but execCtx.Err() is still DeadlineExceeded.
func einoExecuteRecvErrIsToolTimeout(rerr error, tctx context.Context) bool {
	if tctx != nil && errors.Is(tctx.Err(), context.DeadlineExceeded) {
		return true
	}
	return errors.Is(rerr, context.DeadlineExceeded)
}

// einoStreamingShellWrap wraps the StreamingShell used by Eino filesystem (cloudwego eino-ext local.Local).
// The official execute tool defaults to ExecuteStreaming without RunInBackendGround; when ending with &, the child process remains attached to the pipe,
// and streamStdout reading line-by-line will block for a long time when there is no newline output (unlike the separate implementation of MCP tool exec).
// Automatically enables RunInBackendGround for 'fully background' commands, aligning with local.runCmdInBackground behavior.
//
// Uses a Pipe to forward the inner stream to the caller: synchronously calls ToolInvokeNotify.Fire after inner EOF but before closing the Pipe,
// so the run loop pushes tool_result immediately upon Fire (toolResultSent deduplicates), preventing the UI from sticking on 'executing' when ADK Tool events arrive late.
//
// If inner returns an error directly during validation (before a reader is established), the goroutine below is not entered; Fire must still be called;
// otherwise the pending tool_call waits until the entire run ends to be force-closed, out of sync with the soft error text already shown for assistant/tool.
type einoStreamingShellWrap struct {
	inner         filesystem.StreamingShell
	invokeNotify  *einomcp.ToolInvokeNotifyHolder
	einoAgentName string
	// outputChunk is optional; when non-nil, push each received inner ExecuteResponse chunk, consistent with MCP tool's tool_result_delta (requires a valid toolCallId).
	outputChunk func(toolName, toolCallID, chunk string)
	// toolTimeoutMinutes aligns with agent.tool_timeout_minutes; when >0, applies a context timeout to a single execute call (like MCP tool via executeToolViaMCP 行为一致）。0 表示仅依赖上层 ctx（如整task 10h 上限）。
	toolTimeoutMinutes int
	// toolWaitTimeoutSeconds aligns with agent.tool_wait_timeout_seconds; when >0, returns execution_id after this wait expires while the shell continues running in the background.
	toolWaitTimeoutSeconds int
	// shellNoOutputTimeoutSec: idle seconds with no output; 0 = disabled.
	shellNoOutputTimeoutSec int
	// beginMonitor writes running status when execute starts; finishMonitor updates to completed/failed after the stream ends.
	beginMonitor            func(toolCallID, command string) string
	appendPartialMonitor    func(executionID, toolCallID, chunk string)
	registerCancelMonitor   func(executionID string, cancel context.CancelFunc)
	unregisterCancelMonitor func(executionID string)
	finishMonitor           func(executionID, toolCallID, command, stdout string, success bool, invokeErr error)
}

func (w *einoStreamingShellWrap) ExecuteStreaming(ctx context.Context, input *filesystem.ExecuteRequest) (*schema.StreamReader[*filesystem.ExecuteResponse], error) {
	if w.inner == nil {
		return nil, fmt.Errorf("einoStreamingShellWrap: inner shell is nil")
	}
	if input == nil {
		return w.inner.ExecuteStreaming(ctx, nil)
	}
	req := *input
	userCmd := strings.TrimSpace(req.Command)
	tid := strings.TrimSpace(compose.GetToolCallID(ctx))
	agentTag := strings.TrimSpace(w.einoAgentName)
	if security.IsBackgroundShellCommand(req.Command) && !req.RunInBackendGround {
		req.RunInBackendGround = true
	}
	req.Command = prependPythonUnbufferedEnv(req.Command)
	convID := mcp.MCPConversationIDFromContext(ctx)
	execReg := mcp.EinoExecuteRunRegistryFromContext(ctx)

	var monitorExecID string
	if w.beginMonitor != nil {
		monitorExecID = w.beginMonitor(tid, userCmd)
	}
	if monitorExecID != "" && convID != "" {
		if toolReg := mcp.ToolRunRegistryFromContext(ctx); toolReg != nil {
			toolReg.RegisterRunningTool(convID, monitorExecID)
		}
	}
	toolRunReg := mcp.ToolRunRegistryFromContext(ctx)

	execCtx, execCancel := context.WithCancel(ctx)
	var timeoutCancel context.CancelFunc
	if w.toolTimeoutMinutes > 0 {
		execCtx, timeoutCancel = context.WithTimeout(execCtx, time.Duration(w.toolTimeoutMinutes)*time.Minute)
	}
	if monitorExecID != "" && w.registerCancelMonitor != nil {
		w.registerCancelMonitor(monitorExecID, execCancel)
	}
	if execReg != nil && convID != "" {
		execReg.RegisterActiveEinoExecute(convID, execCancel)
	}

	sr, err := w.inner.ExecuteStreaming(execCtx, &req)
	if err != nil {
		if timeoutCancel != nil {
			timeoutCancel()
		}
		if execCancel != nil {
			execCancel()
		}
		if monitorExecID != "" && w.unregisterCancelMonitor != nil {
			w.unregisterCancelMonitor(monitorExecID)
		}
		if einoExecuteRecvErrIsToolTimeout(err, execCtx) {
			hint := "\n\n" + einoExecuteTimeoutUserHint() + "\n"
			if w.finishMonitor != nil {
				w.finishMonitor(monitorExecID, tid, userCmd, hint, false, context.DeadlineExceeded)
			}
			if w.invokeNotify != nil && tid != "" {
				w.invokeNotify.Fire(tid, "execute", agentTag, false, hint, context.DeadlineExceeded)
			}
			return schema.StreamReaderFromArray([]*filesystem.ExecuteResponse{{Output: hint}}), nil
		}
		if w.finishMonitor != nil {
			w.finishMonitor(monitorExecID, tid, userCmd, "", false, err)
		}
		if w.invokeNotify != nil && tid != "" {
			w.invokeNotify.Fire(tid, "execute", agentTag, false, "", err)
		}
		return nil, err
	}
	if sr == nil {
		if timeoutCancel != nil {
			timeoutCancel()
		}
		if execCancel != nil {
			execCancel()
		}
		return sr, nil
	}

	outR, outW := schema.Pipe[*filesystem.ExecuteResponse](32)

	go func(inner *schema.StreamReader[*filesystem.ExecuteResponse], command string, cancel context.CancelFunc, timeoutCleanup context.CancelFunc, tctx context.Context, conversationID string, reg mcp.EinoExecuteRunRegistry, toolReg mcp.ToolRunRegistry, execID string, toolCallID string, noOutputSec int, waitTimeoutSec int) {
		var innerCloseOnce sync.Once
		closeInner := func() {
			innerCloseOnce.Do(func() { inner.Close() })
		}
		defer closeInner()
		if timeoutCleanup != nil {
			defer timeoutCleanup()
		}
		if cancel != nil {
			defer cancel()
		}
		if reg != nil && conversationID != "" {
			defer reg.UnregisterActiveEinoExecute(conversationID)
		}
		if toolReg != nil && conversationID != "" && execID != "" {
			defer toolReg.UnregisterRunningTool(conversationID, execID)
		}
		if w.unregisterCancelMonitor != nil && execID != "" {
			defer w.unregisterCancelMonitor(execID)
		}

		// when ctx is cancelled, close the inner stream to avoid Recv permanently blocking when tools like amass produce no newline output for a long time.
		stopWatch := make(chan struct{})
		go func() {
			select {
			case <-tctx.Done():
				closeInner()
			case <-stopWatch:
			}
		}()
		defer close(stopWatch)

		var sb strings.Builder
		success := true
		var invokeErr error
		exitCode := 0
		hasExitCode := false
		softReturned := false
		var outCloseOnce sync.Once
		closeOut := func() {
			outCloseOnce.Do(func() { outW.Close() })
		}
		defer closeOut()
		sendOut := func(resp *filesystem.ExecuteResponse, err error) bool {
			if softReturned {
				return false
			}
			return outW.Send(resp, err)
		}

		idleWatch := security.NewShellInactivityWatch(noOutputSec)
		if idleWatch != nil {
			defer idleWatch.Stop()
		}
		var waitTimeoutCh <-chan time.Time
		var waitTimer *time.Timer
		if waitTimeoutSec > 0 {
			waitTimer = time.NewTimer(time.Duration(waitTimeoutSec) * time.Second)
			waitTimeoutCh = waitTimer.C
			defer waitTimer.Stop()
		}

		type execRecvMsg struct {
			resp *filesystem.ExecuteResponse
			err  error
		}
		recvCh := make(chan execRecvMsg, 1)
		go func() {
			for {
				resp, rerr := inner.Recv()
				recvCh <- execRecvMsg{resp: resp, err: rerr}
				if rerr != nil {
					return
				}
			}
		}()

		fireInactivityTimeout := func() {
			success = false
			invokeErr = fmt.Errorf("shell inactivity timeout (%ds)", idleWatch.Sec)
			msg := security.ShellNoOutputTimeoutMessage(idleWatch.Sec)
			_ = sendOut(&filesystem.ExecuteResponse{Output: msg}, nil)
			sb.WriteString(msg)
			if w.appendPartialMonitor != nil && execID != "" {
				w.appendPartialMonitor(execID, toolCallID, msg)
			}
			if w.outputChunk != nil && toolCallID != "" {
				w.outputChunk("execute", toolCallID, msg)
			}
			if cancel != nil {
				cancel()
			}
			closeInner()
		}

	recvLoop:
		for {
			var idleCh <-chan struct{}
			if idleWatch != nil {
				idleCh = idleWatch.Expired
			}
			select {
			case <-idleCh:
				fireInactivityTimeout()
				break recvLoop
			case <-waitTimeoutCh:
				if execID != "" && !softReturned {
					msg := einoExecuteSoftWaitTimeoutResult(execID, waitTimeoutSec)
					_ = outW.Send(&filesystem.ExecuteResponse{Output: msg}, nil)
					softReturned = true
					closeOut()
				}
				waitTimeoutCh = nil
			case msg := <-recvCh:
				rerr := msg.err
				resp := msg.resp
				if errors.Is(rerr, io.EOF) {
					break recvLoop
				}
				if rerr != nil {
					success = false
					invokeErr = rerr
					if einoExecuteRecvErrIsToolTimeout(rerr, tctx) {
						invokeErr = context.DeadlineExceeded
						break recvLoop
					}
					if errors.Is(rerr, context.Canceled) || (tctx != nil && errors.Is(tctx.Err(), context.Canceled)) {
						invokeErr = context.Canceled
						break recvLoop
					}
					_ = sendOut(nil, rerr)
					break recvLoop
				}
				if resp != nil {
					if resp.ExitCode != nil {
						hasExitCode = true
						exitCode = *resp.ExitCode
						continue
					}
					var appended string
					if resp.Output != "" {
						if security.IsLegacyShellExitNoise(resp.Output) {
							continue
						}
						if idleWatch != nil {
							idleWatch.Bump()
						}
						sb.WriteString(resp.Output)
						appended = resp.Output
						if w.appendPartialMonitor != nil && execID != "" {
							w.appendPartialMonitor(execID, toolCallID, appended)
						}
					}
					if w.outputChunk != nil && strings.TrimSpace(appended) != "" {
						w.outputChunk("execute", toolCallID, appended)
					}
					if sendOut(resp, nil) {
						success = false
						invokeErr = fmt.Errorf("execute stream closed by consumer")
						break recvLoop
					}
				}
			}
		}

		if success && hasExitCode && exitCode != 0 {
			success = false
			invokeErr = &ExecuteExitError{Code: exitCode}
		}
		// After WithTimeout fires, the child process is often ended by a signal; the local side typically reports exit -1 / canceled; the error chain may not contain DeadlineExceeded.
		// Normalize using the execution ctx so the UI shows 'timed out' rather than an ambiguous -1.
		if tctx != nil && errors.Is(tctx.Err(), context.DeadlineExceeded) {
			success = false
			invokeErr = context.DeadlineExceeded
		}
		// user 'interrupt and continue' terminates execute: merge the note into the tool result (consistent with MCP CancelToolExecutionWithNote).
		partialStreamed := sb.String()
		var abortNote string
		if reg != nil && conversationID != "" && (invokeErr != nil || errors.Is(tctx.Err(), context.Canceled)) {
			if note := reg.TakeEinoExecuteAbortNote(conversationID); note != "" {
				abortNote = note
				merged := mcp.MergePartialToolOutputAndAbortNote(partialStreamed, note)
				sb.Reset()
				sb.WriteString(merged)
				if invokeErr == nil {
					success = false
					invokeErr = context.Canceled
				}
			}
		}
		// ADK builds the tool message body from this Pipe; only the Notify tail marker does not enter the model context. The timeout message is written to the stream, consistent with the UI.
		if invokeErr != nil && errors.Is(invokeErr, context.DeadlineExceeded) {
			hint := "\n\n" + einoExecuteTimeoutUserHint() + "\n"
			_ = sendOut(&filesystem.ExecuteResponse{Output: hint}, nil)
			if w.appendPartialMonitor != nil && execID != "" {
				w.appendPartialMonitor(execID, toolCallID, hint)
			}
			if w.outputChunk != nil && tid != "" {
				w.outputChunk("execute", tid, hint)
			}
			sb.WriteString(hint)
		}
		// During interruption the loop has already written stdout line by line; only append USER INTERRUPT NOTE here to avoid duplicating the full output.
		if invokeErr != nil && errors.Is(invokeErr, context.Canceled) && abortNote != "" {
			if partialStreamed != "" {
				_ = sendOut(&filesystem.ExecuteResponse{Output: "\n\n" + mcp.AbortNoteBannerForModel + "\n" + abortNote}, nil)
			} else if text := strings.TrimSpace(sb.String()); text != "" {
				_ = sendOut(&filesystem.ExecuteResponse{Output: text + "\n"}, nil)
			}
		}
		rawOutput := sb.String()
		fireBody := rawOutput
		if !success && hasExitCode && exitCode != 0 {
			statusLine := security.ExecuteFailureStatusLine(exitCode)
			if !strings.Contains(rawOutput, "command execution failed:") {
				_ = sendOut(&filesystem.ExecuteResponse{Output: statusLine}, nil)
				if w.appendPartialMonitor != nil && execID != "" {
					w.appendPartialMonitor(execID, toolCallID, statusLine)
				}
				sb.WriteString(statusLine)
			}
			fireBody = einomcp.ToolErrorPrefix + security.FormatCommandFailureResult(exitCode, rawOutput)
		}
		if w.finishMonitor != nil {
			w.finishMonitor(execID, toolCallID, command, sb.String(), success, invokeErr)
		}
		if w.invokeNotify != nil {
			if !softReturned {
				w.invokeNotify.Fire(toolCallID, "execute", agentTag, success, fireBody, invokeErr)
			}
		}
	}(sr, userCmd, execCancel, timeoutCancel, execCtx, convID, execReg, toolRunReg, monitorExecID, tid, w.shellNoOutputTimeoutSec, w.toolWaitTimeoutSeconds)

	return outR, nil
}

func einoExecuteSoftWaitTimeoutResult(executionID string, waitTimeoutSec int) string {
	waitText := "configured wait timeout"
	if waitTimeoutSec > 0 {
		waitText = fmt.Sprintf("%ds", waitTimeoutSec)
	}
	return fmt.Sprintf(`Tool submitted to background execution and is still running.

execution_id: %s
status: running
wait_timeout: %s

You may continue reasoning, switch to another tool, or call get_tool_execution / wait_tool_execution to read partial_output and continue waiting; you may also call cancel_tool_execution cancelled。`, executionID, waitText)
}
