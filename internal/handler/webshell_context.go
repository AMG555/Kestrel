package handler

import (
	"strings"

	"kestrel/internal/database"
)

// WebshellSkillHintDefault is the Skills description shared by the conversation page / Eino single-agent, placed at the end of the webshell context,
// for AI to reference when choosing a skill loading entry point.
const WebshellSkillHintDefault = "Please use the built-in `skill` tool in a multi-agent / Eino DeepAgent conversation to progressively load Skill packages."

// WebshellSkillHintMultiAgent is the Skills description used during the multi-agent / Eino multi-agent preparation phase
const WebshellSkillHintMultiAgent = "Please use the Eino multi-agent built-in `skill` tool for Skill packages."

// webshellAssistantToolList is the list of tools allowed for the AI assistant in the WebShell context (displayed to the model).
// Note: this is only a display string; actual permission restrictions are in the roleTools slice set by the caller.
const webshellAssistantToolList = "webshell_exec、webshell_file_list、webshell_file_read、webshell_file_write、record_vulnerability、list_vulnerabilities、get_vulnerability、upsert_project_fact、get_project_fact、list_project_facts、search_project_facts、deprecate_project_fact、restore_project_fact、list_knowledge_risk_types、search_knowledge_base"

// BuildWebshellAssistantContext assembles the AI assistant context prompt from the connection info and user's original message.
// Context includes: connection ID, Remark, target system (with recommended command set), response encoding, available tool list, Skills loading entry point,
// and the final user request. The caller only needs to decide the skillHint text (defaults to WebshellSkillHintDefault).
//
// This logic is extracted into a shared function to avoid copy-pasting across agent.go / multi_agent_prepare.go and other files,
// and to ensure that upgrading OS / Encoding descriptions only requires changing one place, testing one place, and syncing.
func BuildWebshellAssistantContext(conn *database.WebShellConnection, skillHint, userMsg string) string {
	if conn == nil {
		// fallback: caller guarantees conn is non-nil; this is just a defensive return of the original message
		return userMsg
	}
	remark := conn.Remark
	if remark == "" {
		remark = conn.URL
	}

	targetOS := resolveWebshellOS(conn.OS, conn.Type) // normalized to "linux" / "windows"
	encoding := normalizeWebshellEncoding(conn.Encoding)
	if skillHint == "" {
		skillHint = WebshellSkillHintDefault
	}

	var b strings.Builder
	b.Grow(512 + len(userMsg))

	b.WriteString("[WebShell Assistant Context] Connection ID: ")
	b.WriteString(conn.ID)
	b.WriteString(", Remark: ")
	b.WriteString(remark)
	b.WriteByte('\n')

	// target system: explicitly tell the AI which command set to use/avoid, so it doesn't send ls/cat/rm to Windows
	b.WriteString("- Target system: ")
	b.WriteString(describeTargetOSForPrompt(targetOS))
	b.WriteByte('\n')

	// response encoding: only explicitly inform when non-auto; auto mode is handled by backend self-adaptation without disturbing the model
	if encHint := describeEncodingForPrompt(encoding); encHint != "" {
		b.WriteString("- Response encoding: ")
		b.WriteString(encHint)
		b.WriteByte('\n')
	}

	// tool list & connection_id constraint: keep the existing phrasing; the AI is already familiar with it
	b.WriteString("Available tools (use only for operations on this connection, set connection_id to \"")
	b.WriteString(conn.ID)
	b.WriteString("\"): ")
	b.WriteString(webshellAssistantToolList)
	b.WriteString(". Record as you pentest: call upsert_project_fact on every confirmed new finding, call record_vulnerability on every validated vulnerability; do not wait until the session ends.")
	b.WriteString(skillHint)
	b.WriteString("\n\nUser request: ")
	b.WriteString(userMsg)

	return b.String()
}

// describeTargetOSForPrompt returns a description + recommended command set + counter-examples for the given OS,
// command list covers the 6 most common file management actions (view/read/delete/rename/mkdir/find), so the AI can copy them directly.
func describeTargetOSForPrompt(targetOS string) string {
	switch targetOS {
	case "windows":
		return "Windows (recommended cmd/PowerShell: dir /a, type, del /q /f, move /y, md, ren;" +
			"find files with `dir /s /b filterword` or PowerShell `Get-ChildItem -Recurse`;" +
			"avoid Unix commands like ls / cat / rm / mv / find, otherwise the response will be `is not recognized as an internal or external command`)"
	case "linux":
		return "Linux/Unix (recommended sh/bash: ls -la, cat, rm -f, mv, mkdir -p;" +
			"find files with `find /path -name '*pattern*'`;" +
			"avoid Windows commands like dir, type, del, move)"
	default:
		// theoretically unreachable; resolveWebshellOS provides the fallback
		return "unknown (please run `uname || ver` to detect OS before choosing a command set)"
	}
}

// describeEncodingForPrompt returns a human-readable description of the response encoding; auto returns empty string to reduce tokens.
func describeEncodingForPrompt(encoding string) string {
	switch encoding {
	case "utf-8":
		return "UTF-8 (target natively uses UTF-8, no extra decoding needed)"
	case "gbk":
		return "GBK (Chinese Windows; backend auto-transcoded to UTF-8; if \\uFFFD replacement characters still appear, the command failed or encoding detection errored)"
	case "gb18030":
		return "GB18030 (backend auto-transcoded to UTF-8)"
	default:
		return ""
	}
}
