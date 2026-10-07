package multiagent

import (
	"strings"

	"kestrel/internal/projectprompt"
)

func shellToolsPresent(toolNames []string) bool {
	for _, n := range toolNames {
		switch strings.ToLower(strings.TrimSpace(n)) {
		case "exec", "execute":
			return true
		}
	}
	return false
}

// injectShellToolGuidance 在system prompt末尾追加 exec/execute 分工（仅当tool list含 exec 或 execute）。
func injectShellToolGuidance(instruction string, toolNames []string) string {
	if !shellToolsPresent(toolNames) {
		return instruction
	}
	block := strings.TrimSpace(projectprompt.ShellExecExecuteGuidanceSection())
	if block == "" {
		return instruction
	}
	s := strings.TrimSpace(instruction)
	if s == "" {
		return block
	}
	return s + "\n\n" + block
}
