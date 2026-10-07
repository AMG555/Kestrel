package einomcp

import "sync"

// ToolInvokeNotifyHolder is shared between the Eino run loop and the MCP/execute bridge; Fire is triggered on the raw tool return.
// The UI's tool_result must wait for the ADK schema.Tool event (post-reduction body) and is not pushed from this holder's callback.
type ToolInvokeNotifyHolder struct {
	mu sync.RWMutex
	fn func(toolCallID, toolName, einoAgent string, success bool, content string, invokeErr error)
}

// NewToolInvokeNotifyHolder creates a holder shareable between ToolsFromDefinitions and the run loop.
func NewToolInvokeNotifyHolder() *ToolInvokeNotifyHolder {
	return &ToolInvokeNotifyHolder{}
}

// Set is called by runEinoADKAgentLoop before it begins consuming the iter; can be overwritten multiple times (usually just once).
func (h *ToolInvokeNotifyHolder) Set(fn func(toolCallID, toolName, einoAgent string, success bool, content string, invokeErr error)) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fn = fn
}

// Fire is called by mcpBridgeTool on tool callback; ignored if Set has not been called or if toolCallID is empty.
func (h *ToolInvokeNotifyHolder) Fire(toolCallID, toolName, einoAgent string, success bool, content string, invokeErr error) {
	if h == nil {
		return
	}
	h.mu.RLock()
	fn := h.fn
	h.mu.RUnlock()
	if fn == nil {
		return
	}
	fn(toolCallID, toolName, einoAgent, success, content, invokeErr)
}
