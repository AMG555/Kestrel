package workflow

import (
	"context"

	"kestrel/internal/config"
	"kestrel/internal/einoobserve"
)

func attachWorkflowCallbacks(ctx context.Context, cfg *config.Config, args RunArgs, workflowName string) context.Context {
	if cfg == nil {
		return ctx
	}
	cbCfg := &cfg.MultiAgent.EinoCallbacks
	return einoobserve.AttachAgentRunCallbacks(ctx, cbCfg, einoobserve.Params{
		Logger:           args.Logger,
		Progress:         args.Progress,
		ConversationID:   args.ConversationID,
		OrchMode:         "workflow",
		OrchestratorName: workflowName,
	})
}
