package multiagent

import "kestrel/internal/config"

const defaultAgentMaxIterations = 3000

// agentMaxIterations returns the global iteration cap: uses only config.agent.max_iterations; when ≤0 defaults to 3000.
func agentMaxIterations(appCfg *config.Config) int {
	if appCfg != nil && appCfg.Agent.MaxIterations > 0 {
		return appCfg.Agent.MaxIterations
	}
	return defaultAgentMaxIterations
}

// resolveMaxIterations returns the unified iteration cap: max_iterations>0 in Markdown/sub-agent front matter overrides per-agent; otherwise uses agent.max_iterations.
// multi_agent.max_iteration and sub_agent_max_iterations are deprecated and no longer considered.
func resolveMaxIterations(appCfg *config.Config, markdownOverride int) int {
	if markdownOverride > 0 {
		return markdownOverride
	}
	return agentMaxIterations(appCfg)
}
