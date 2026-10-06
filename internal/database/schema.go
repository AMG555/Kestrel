package database

import "fmt"

// initSchema creates all tables and indexes if they don't already exist.
func (db *DB) initSchema() error {
	stmts := []struct {
		name string
		ddl  string
	}{
		{"users", `CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			must_change_password INTEGER NOT NULL DEFAULT 1,
			is_active INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`},
		{"roles", `CREATE TABLE IF NOT EXISTS roles (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			is_system INTEGER NOT NULL DEFAULT 0,
			permissions_json TEXT NOT NULL DEFAULT '[]',
			allowed_tools_json TEXT NOT NULL DEFAULT '[]',
			hitl_mode TEXT NOT NULL DEFAULT 'auto',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`},
		{"user_roles", `CREATE TABLE IF NOT EXISTS user_roles (
			user_id TEXT NOT NULL,
			role_id TEXT NOT NULL,
			assigned_at DATETIME NOT NULL,
			assigned_by TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (user_id, role_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
			FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE CASCADE
		)`},
		{"sessions", `CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			ip_address TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`},
		{"audit_logs", `CREATE TABLE IF NOT EXISTS audit_logs (
			id TEXT PRIMARY KEY,
			created_at DATETIME NOT NULL,
			actor_id TEXT NOT NULL DEFAULT '',
			actor_name TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			category TEXT NOT NULL,
			result TEXT NOT NULL,
			resource_type TEXT NOT NULL DEFAULT '',
			resource_id TEXT NOT NULL DEFAULT '',
			client_ip TEXT NOT NULL DEFAULT '',
			user_agent TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL,
			detail_json TEXT NOT NULL DEFAULT '{}'
		)`},
		{"projects", `CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			scope_json TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'active',
			owner_user_id TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`},
		{"assets", `CREATE TABLE IF NOT EXISTS assets (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			dedup_key TEXT NOT NULL UNIQUE,
			host TEXT NOT NULL DEFAULT '',
			ip TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			domain TEXT NOT NULL DEFAULT '',
			protocol TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			service TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			owner_user_id TEXT NOT NULL DEFAULT '',
			tags_json TEXT NOT NULL DEFAULT '[]',
			vulnerability_count INTEGER NOT NULL DEFAULT 0,
			risk_level TEXT NOT NULL DEFAULT 'unassessed',
			source TEXT NOT NULL DEFAULT 'manual',
			first_seen_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL
		)`},
		{"vulnerabilities", `CREATE TABLE IF NOT EXISTS vulnerabilities (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			asset_id TEXT,
			session_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			severity TEXT NOT NULL DEFAULT 'info',
			status TEXT NOT NULL DEFAULT 'open',
			vuln_type TEXT NOT NULL DEFAULT '',
			target TEXT NOT NULL DEFAULT '',
			reproduction_steps TEXT NOT NULL DEFAULT '',
			evidence TEXT NOT NULL DEFAULT '',
			recommendation TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL,
			FOREIGN KEY (asset_id) REFERENCES assets(id) ON DELETE SET NULL
		)`},
		{"sessions_agent", `CREATE TABLE IF NOT EXISTS agent_sessions (
			id TEXT PRIMARY KEY,
			project_id TEXT,
			user_id TEXT NOT NULL,
			role_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			agent_mode TEXT NOT NULL DEFAULT 'single',
			status TEXT NOT NULL DEFAULT 'active',
			hitl_mode TEXT NOT NULL DEFAULT 'auto',
			pinned INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE SET NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`},
		{"messages", `CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			reasoning_content TEXT NOT NULL DEFAULT '',
			tool_call_ids TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME NOT NULL,
			FOREIGN KEY (session_id) REFERENCES agent_sessions(id) ON DELETE CASCADE
		)`},
		{"tool_executions", `CREATE TABLE IF NOT EXISTS tool_executions (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL DEFAULT '',
			message_id TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			tool_name TEXT NOT NULL,
			arguments_json TEXT NOT NULL DEFAULT '{}',
			status TEXT NOT NULL DEFAULT 'pending',
			result TEXT,
			error TEXT NOT NULL DEFAULT '',
			output_bytes INTEGER NOT NULL DEFAULT 0,
			output_truncated INTEGER NOT NULL DEFAULT 0,
			hitl_required INTEGER NOT NULL DEFAULT 0,
			hitl_approved INTEGER,
			hitl_approver_id TEXT NOT NULL DEFAULT '',
			hitl_decided_at DATETIME,
			started_at DATETIME NOT NULL,
			completed_at DATETIME,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`},
		{"mcp_servers", `CREATE TABLE IF NOT EXISTS mcp_servers (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			transport TEXT NOT NULL DEFAULT 'http',
			url TEXT NOT NULL DEFAULT '',
			command_json TEXT NOT NULL DEFAULT '[]',
			headers_json TEXT NOT NULL DEFAULT '{}',
			timeout_seconds INTEGER NOT NULL DEFAULT 120,
			is_active INTEGER NOT NULL DEFAULT 1,
			allowed_tools_json TEXT NOT NULL DEFAULT '[]',
			circuit_open INTEGER NOT NULL DEFAULT 0,
			failure_count INTEGER NOT NULL DEFAULT 0,
			last_failure_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`},
		{"workflow_definitions", `CREATE TABLE IF NOT EXISTS workflow_definitions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			graph_json TEXT NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`},
		{"workflow_runs", `CREATE TABLE IF NOT EXISTS workflow_runs (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			session_id TEXT,
			project_id TEXT,
			user_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'running',
			input_json TEXT NOT NULL DEFAULT '{}',
			output_json TEXT,
			error TEXT NOT NULL DEFAULT '',
			started_at DATETIME NOT NULL,
			finished_at DATETIME,
			FOREIGN KEY (workflow_id) REFERENCES workflow_definitions(id) ON DELETE CASCADE
		)`},
		{"knowledge_documents", `CREATE TABLE IF NOT EXISTS knowledge_documents (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			source_path TEXT NOT NULL DEFAULT '',
			content_hash TEXT NOT NULL DEFAULT '',
			chunk_count INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`},
		{"knowledge_chunks", `CREATE TABLE IF NOT EXISTS knowledge_chunks (
			id TEXT PRIMARY KEY,
			document_id TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			content TEXT NOT NULL,
			embedding_json TEXT,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (document_id) REFERENCES knowledge_documents(id) ON DELETE CASCADE
		)`},
	}

	for _, s := range stmts {
		if _, err := db.Exec(s.ddl); err != nil {
			return fmt.Errorf("creating table %q: %w", s.name, err)
		}
	}

	return db.initIndexes()
}

func (db *DB) initIndexes() error {
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_id ON audit_logs(actor_id)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_logs_category ON audit_logs(category)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_project_id ON assets(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_ip ON assets(ip)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_domain ON assets(domain)`,
		`CREATE INDEX IF NOT EXISTS idx_assets_status ON assets(status)`,
		`CREATE INDEX IF NOT EXISTS idx_vulns_project_id ON vulnerabilities(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vulns_asset_id ON vulnerabilities(asset_id)`,
		`CREATE INDEX IF NOT EXISTS idx_vulns_severity ON vulnerabilities(severity)`,
		`CREATE INDEX IF NOT EXISTS idx_vulns_status ON vulnerabilities(status)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_user_id ON agent_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_sessions_project_id ON agent_sessions(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_exec_session_id ON tool_executions(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_exec_tool_name ON tool_executions(tool_name)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_exec_status ON tool_executions(status)`,
		`CREATE INDEX IF NOT EXISTS idx_user_roles_user_id ON user_roles(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_roles_role_id ON user_roles(role_id)`,
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("creating index: %w", err)
		}
	}
	return nil
}
