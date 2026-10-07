package multiagent

import "errors"

// ErrInterruptContinue 作为 context.CancelCause 使用：user选择「中断并continue」且当前nonein progress的 MCP tool时，
// cancelled当前推理/streaming output，并在同一会话task内携带user补充说明Auto-continue下一轮（类似 Hermes 式人机回合）。
var ErrInterruptContinue = errors.New("agent interrupt: continue with user-supplied context")
