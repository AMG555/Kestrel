package c2

import "context"

type hitlRunCtxKey struct{}

// WithHITLRunContext attaches runCtx (typically the lifetime of the entire Agent / SSE request) to the given ctx.
// The ctx received by an MCP tool handler may be a child context with a single-tool timeout that gets cancelled when the tool returns;
// dangerous task HITL should use runCtx via HITLUserContext to wait for human approval.
func WithHITLRunContext(ctx, runCtx context.Context) context.Context {
	if ctx == nil || runCtx == nil {
		return ctx
	}
	return context.WithValue(ctx, hitlRunCtxKey{}, runCtx)
}

// HITLUserContext returns the context for C2 dangerous task HITL waiting:
// if a longer-lived runCtx was previously injected via WithHITLRunContext, it is returned; otherwise ctx is returned.
func HITLUserContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if v := ctx.Value(hitlRunCtxKey{}); v != nil {
		if run, ok := v.(context.Context); ok && run != nil {
			return run
		}
	}
	return ctx
}
