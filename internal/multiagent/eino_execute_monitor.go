package multiagent

import (
	"context"
	"fmt"

	"kestrel/internal/agent"
	"kestrel/internal/einomcp"
)

// newEinoExecuteMonitorCallbacks writes to the MCP monitor library and calls recorder(executionId)
// when an Eino filesystem execute starts/ends, consistent with the CallTool path so the monitor
// page can display "running" status.
func newEinoExecuteMonitorCallbacks(ctx context.Context, ag *agent.Agent, recorder einomcp.ExecutionRecorder) (
	begin func(toolCallID, command string) string,
	appendPartial func(executionID, toolCallID, chunk string),
	registerCancel func(executionID string, cancel context.CancelFunc),
	unregisterCancel func(executionID string),
	finish func(executionID, toolCallID, command, stdout string, success bool, invokeErr error),
) {
	begin = func(toolCallID, command string) string {
		if ag == nil {
			return ""
		}
		args := map[string]interface{}{"command": command}
		id := ag.BeginLocalToolExecution(ctx, "execute", args)
		if id != "" && recorder != nil {
			recorder(id, toolCallID)
		}
		return id
	}
	appendPartial = func(executionID, toolCallID, chunk string) {
		if ag == nil || executionID == "" || chunk == "" {
			return
		}
		ag.AppendLocalToolExecutionPartialOutput(executionID, chunk)
	}
	registerCancel = func(executionID string, cancel context.CancelFunc) {
		if ag == nil || executionID == "" || cancel == nil {
			return
		}
		ag.RegisterLocalToolExecutionCancel(executionID, cancel)
	}
	unregisterCancel = func(executionID string) {
		if ag == nil || executionID == "" {
			return
		}
		ag.UnregisterLocalToolExecutionCancel(executionID)
	}
	finish = func(executionID, toolCallID, command, stdout string, success bool, invokeErr error) {
		if ag == nil {
			return
		}
		var err error
		if !success {
			if invokeErr != nil {
				err = invokeErr
			} else {
				err = fmt.Errorf("execute failed")
			}
		}
		args := map[string]interface{}{"command": command}
		id := ag.FinishLocalToolExecution(ctx, executionID, "execute", args, stdout, err)
		if id != "" && recorder != nil && executionID == "" {
			recorder(id, toolCallID)
		}
	}
	return begin, appendPartial, registerCancel, unregisterCancel, finish
}
