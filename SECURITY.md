# Security Policy

## Overview

Kestrel is an AI-native security operations and automated evaluation platform. It executes authorized diagnostic tools, integrates Model Context Protocol (MCP) servers, manages authorized adversary emulation listeners, and orchestrates verification workflows. Treat every deployment as a high-privilege operational environment.

### Supported Versions

Security updates are committed to the latest repository release and default branch. When reporting an issue, verify against the current release or mainline branch where possible.

### Reporting a Vulnerability

Do not publicly disclose security vulnerabilities before the development team has reviewed and verified the report.

Recommended disclosure content:
- Affected version, commit hash, or deployment environment
- Execution mode and relevant configuration flags
- Minimal reproducible proof-of-concept steps
- Security impact assessment (CVSS rating or business impact)
- Affected module: authentication, authorization, agent execution, MCP boundary, ToolGuard, or web API
- Whether authentication or specific role permissions are required
- Recommended mitigation or remediation steps

### Scope

In-scope items:
- Authentication, session validation, and token signing flaws
- Role-based access control (RBAC) and authorization bypasses
- Command injection outside designed tool execution sandboxes
- Path traversal or unintended file read/write vulnerabilities
- MCP boundary breaks or untrusted tool input escalation
- Credential leakage in logs, audit records, or API responses
- Cross-Site Scripting (XSS) or CSRF in the web interface
- Unsafe deserialization or memory safety vulnerabilities

Out-of-scope items:
- Attacks targeting systems without explicit written penetration testing authorization
- Denial-of-service (DoS/DDoS) against public infrastructures
- Social engineering, phishing, or credential harvesting against operators
- Flaws arising exclusively from explicitly disabling security controls in `config.yaml`
- Inherent vulnerabilities in external 3rd-party binary tools (e.g. nmap, sqlmap) unless exacerbated by Kestrel

### Authorized Testing Boundary

Kestrel must only be deployed and executed against assets and systems owned by your organization or for which explicit written authorization has been granted. Unauthorized use is strictly prohibited.

### Hardening Recommendations

Before deploying Kestrel in production or shared network environments:
1. Change default administrator credentials immediately upon first initialization.
2. Enable TLS/HTTPS termination with valid certificates.
3. Place Kestrel behind a firewall, VPN, or bastion network.
4. Ensure audit logging is active and forwarded to your central SIEM.
5. Require Human-in-the-Loop (HITL) approval for destructive or high-risk tool categories.
6. Regularly review and backup `config.yaml` and SQLite database files.
