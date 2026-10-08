// Package audit provides the append-only platform audit log.
// All management actions (auth, config, RBAC, tool-guard changes, etc.) are recorded
// here. Audit records are never modified or deleted by the application.
package audit
