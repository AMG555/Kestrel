package projectprompt

// ShellExecExecuteGuidanceSection appends exec/execute role separation guidance for single/multi-agent system prompts (kept concise).
func ShellExecExecuteGuidanceSection() string {
	return `Shell (exec/execute): when a dedicated MCP tool is available, prefer it; use exec for system commands (pipes, workdir, background &); use execute for scripts under skills/ (paired with read_file, skill); split multi-step scans into separate calls — do not chain multiple scanners in one shell command. Long scripts, request bodies, or payloads must first be written to the session working directory with write_file, then executed as short commands with exec/execute; do not embed long content in command. Downloads and temporary files must be written to the "session working directory" in the system prompt — do not use /tmp.`
}

// ShellExecExecuteGuidanceReconSuffix is an optional one-line addition for reconnaissance sub-agents.
func ShellExecExecuteGuidanceReconSuffix() string {
	return `For enumeration, prefer dedicated MCPs such as subfinder or amass — do not chain long commands with exec/execute.`
}
