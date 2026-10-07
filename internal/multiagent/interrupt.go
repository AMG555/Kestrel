package multiagent

import "errors"

// ErrInterruptContinue is used as a context.CancelCause: when the user chooses "interrupt and continue"
// and no MCP tool is currently in progress, it cancels the current reasoning/streaming output
// and auto-continues the next round in the same session task with the user's supplemental note
// (similar to a Hermes-style human-in-the-loop turn).
var ErrInterruptContinue = errors.New("agent interrupt: continue with user-supplied context")
