package projectprompt

import (
	"fmt"
	"strings"
)

// FactRecordingBlackboardSection generates the system prompt block for project blackboard facts and vulnerability logging.
func FactRecordingBlackboardSection() string {
	var b strings.Builder
	b.WriteString("## Project Blackboard (Operational Memory) & Findings Logging\n\n")
	b.WriteString("When operating within a project context, real-time facts and findings must be persisted incrementally:\n\n")
	b.WriteString("- **Incremental Record Rhythm**: Do not wait for session completion. Immediately record newly confirmed facts (open ports, discovered services, technologies, auth endpoints, attack surface changes) using `upsert_project_fact`.\n")
	b.WriteString("- **Reproducible Context**: Always write full context in fact bodies (endpoint URL, headers, parameter keys, observed behavior) so future sessions can retrieve context via `get_project_fact`.\n")
	b.WriteString("- **Formal Vulnerabilities**: When a reproducible vulnerability is verified, call `record_vulnerability` with title, severity (critical, high, medium, low, info), target, proof-of-concept (POC), impact, and remediation guidance.\n")
	b.WriteString("- **Graph Relations**: When logging finding/exploit facts, provide relationship links (e.g., `from: target/*`, `type: discovered_on`, `depends_on`, `leads_to`, `enables`).\n")
	return b.String()
}

// ShellExecGuidanceSection returns system prompt rules for terminal command dispatching.
func ShellExecGuidanceSection() string {
	return fmt.Sprintf(`## Shell & Command Execution Rules
- When dedicated tools exist in the catalog (e.g. nmap, nuclei, subfinder, sqlmap, dirsearch), invoke them directly.
- Multi-step scans must be executed step-by-step; never chain multiple scanners in a single shell pipeline.
- Large scripts or payloads must be written to the session workspace directory first before executing.
- Keep output concise; truncated tool outputs are stored in persisted session spill storage.
`)
}
