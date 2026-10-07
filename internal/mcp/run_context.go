package mcp

import (
	"context"
	"strings"
)

// ToolRunRegistry registers the current executionId when a tool starts/ends.
// Shared by the conversation page "terminate current tool only" and the monitoring page cancellation logic.
type ToolRunRegistry interface {
	RegisterRunningTool(conversationID, executionID string)
	UnregisterRunningTool(conversationID, executionID string)
}

// EinoExecuteRunRegistry registers in-progress Eino filesystem executes,
// used by the "interrupt and continue" flow to terminate long-running commands such as amass.
type EinoExecuteRunRegistry interface {
	RegisterActiveEinoExecute(conversationID string, cancel context.CancelFunc)
	UnregisterActiveEinoExecute(conversationID string)
	AbortActiveEinoExecute(conversationID, note string) bool
	TakeEinoExecuteAbortNote(conversationID string) string
}

type toolRunRegistryCtxKey struct{}
type einoExecuteRunRegistryCtxKey struct{}
type mcpConversationIDCtxKey struct{}
type mcpExecutionIDCtxKey struct{}
type mcpProjectIDCtxKey struct{}

// WithToolRunRegistry injects the registry into ctx (Eino / native Agent task ctx).
func WithToolRunRegistry(ctx context.Context, reg ToolRunRegistry) context.Context {
	if ctx == nil || reg == nil {
		return ctx
	}
	return context.WithValue(ctx, toolRunRegistryCtxKey{}, reg)
}

// ToolRunRegistryFromContext retrieves the registry from ctx (nil if not set).
func ToolRunRegistryFromContext(ctx context.Context) ToolRunRegistry {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(toolRunRegistryCtxKey{}).(ToolRunRegistry)
	return v
}

// WithEinoExecuteRunRegistry injects the Eino execute cancellation registry into ctx.
func WithEinoExecuteRunRegistry(ctx context.Context, reg EinoExecuteRunRegistry) context.Context {
	if ctx == nil || reg == nil {
		return ctx
	}
	return context.WithValue(ctx, einoExecuteRunRegistryCtxKey{}, reg)
}

// EinoExecuteRunRegistryFromContext retrieves the Eino execute registry from ctx (nil if not set).
func EinoExecuteRunRegistryFromContext(ctx context.Context) EinoExecuteRunRegistry {
	if ctx == nil {
		return nil
	}
	v, _ := ctx.Value(einoExecuteRunRegistryCtxKey{}).(EinoExecuteRunRegistry)
	return v
}

// WithMCPConversationID injects the conversation ID into ctx for association with executionId inside CallTool.
func WithMCPConversationID(ctx context.Context, conversationID string) context.Context {
	if ctx == nil {
		return nil
	}
	id := strings.TrimSpace(conversationID)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, mcpConversationIDCtxKey{}, id)
}

// MCPConversationIDFromContext reads the conversation ID from ctx.
func MCPConversationIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(mcpConversationIDCtxKey{}).(string)
	return v
}

// WithMCPExecutionID injects the current tool executionId into ctx for aligning oversized output spill filenames.
func WithMCPExecutionID(ctx context.Context, executionID string) context.Context {
	if ctx == nil {
		return nil
	}
	id := strings.TrimSpace(executionID)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, mcpExecutionIDCtxKey{}, id)
}

// MCPExecutionIDFromContext reads the current tool executionId from ctx.
func MCPExecutionIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(mcpExecutionIDCtxKey{}).(string)
	return v
}

// WithMCPProjectID injects the project ID into ctx for aligning spill paths with project isolation.
func WithMCPProjectID(ctx context.Context, projectID string) context.Context {
	if ctx == nil {
		return nil
	}
	id := strings.TrimSpace(projectID)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, mcpProjectIDCtxKey{}, id)
}

// MCPProjectIDFromContext reads the project ID from ctx.
func MCPProjectIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(mcpProjectIDCtxKey{}).(string)
	return v
}

func notifyToolRunBegin(ctx context.Context, executionID string) {
	reg := ToolRunRegistryFromContext(ctx)
	if reg == nil {
		return
	}
	conv := MCPConversationIDFromContext(ctx)
	if conv == "" || strings.TrimSpace(executionID) == "" {
		return
	}
	reg.RegisterRunningTool(conv, executionID)
}

func notifyToolRunEnd(ctx context.Context, executionID string) {
	reg := ToolRunRegistryFromContext(ctx)
	if reg == nil {
		return
	}
	conv := MCPConversationIDFromContext(ctx)
	if conv == "" || strings.TrimSpace(executionID) == "" {
		return
	}
	reg.UnregisterRunningTool(conv, executionID)
}
