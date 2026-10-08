package database

import (
	"database/sql"
	"strings"
	"time"

	"go.uber.org/zap"
)

// WebShellConnection holds the configuration for a WebShell connection.
type WebShellConnection struct {
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id,omitempty"`
	URL       string    `json:"url"`
	Password  string    `json:"password"`
	Type      string    `json:"type"`
	Method    string    `json:"method"`
	CmdParam  string    `json:"cmdParam"`
	Remark    string    `json:"remark"`
	Encoding  string    `json:"encoding"` // Target response encoding: auto / utf-8 / gbk / gb18030; empty/null defaults to auto.
	OS        string    `json:"os"`       // Target operating system: auto / linux / windows; empty/null/unknown defaults to auto.
	CreatedAt time.Time `json:"createdAt"`
}

// GetWebshellConnectionState returns the persistent state JSON associated with a connection, or "{}" if it does not exist.
func (db *DB) GetWebshellConnectionState(connectionID string) (string, error) {
	var stateJSON string
	err := db.QueryRow(`SELECT state_json FROM webshell_connection_states WHERE connection_id = ?`, connectionID).Scan(&stateJSON)
	if err == sql.ErrNoRows {
		return "{}", nil
	}
	if err != nil {
		db.logger.Error("failed to query WebShell connection state", zap.Error(err), zap.String("connectionID", connectionID))
		return "", err
	}
	if stateJSON == "" {
		stateJSON = "{}"
	}
	return stateJSON, nil
}

// UpsertWebshellConnectionState saves the persistent state JSON associated with a connection.
func (db *DB) UpsertWebshellConnectionState(connectionID, stateJSON string) error {
	if stateJSON == "" {
		stateJSON = "{}"
	}
	query := `
		INSERT INTO webshell_connection_states (connection_id, state_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(connection_id) DO UPDATE SET
			state_json = excluded.state_json,
			updated_at = excluded.updated_at
	`
	if _, err := db.Exec(query, connectionID, stateJSON, time.Now()); err != nil {
		db.logger.Error("save WebShell connection status failed", zap.Error(err), zap.String("connectionID", connectionID))
		return err
	}
	return nil
}

// ListWebshellConnections lists all WebShell connections ordered by creation time descending.
func (db *DB) ListWebshellConnections() ([]WebShellConnection, error) {
	return db.ListWebshellConnectionsForAccess("", "", "")
}

func (db *DB) ListWebshellConnectionsForAccess(userID, scope, projectID string) ([]WebShellConnection, error) {
	query := `
		SELECT id, COALESCE(project_id, '') AS project_id, url, password, type, method, cmd_param, remark,
			COALESCE(encoding, '') AS encoding, COALESCE(os, '') AS os, created_at
		FROM webshell_connections
		WHERE 1=1
	`
	args := []interface{}{}
	projectID = strings.TrimSpace(projectID)
	if projectID == ProjectFilterUnbound {
		query += ` AND COALESCE(project_id, '') = ''`
	} else if projectID != "" {
		query += ` AND COALESCE(project_id, '') = ?`
		args = append(args, projectID)
	}
	userID = strings.TrimSpace(userID)
	if userID != "" && scope != RBACScopeAll {
		query += ` AND (
			owner_user_id = ?
			OR EXISTS (
				SELECT 1 FROM rbac_resource_assignments ra
				WHERE ra.user_id = ? AND ra.resource_type = 'webshell' AND ra.resource_id = webshell_connections.id
			)
		)`
		args = append(args, userID, userID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := db.Query(query, args...)
	if err != nil {
		db.logger.Error("failed to list WebShell connections", zap.Error(err))
		return nil, err
	}
	defer rows.Close()

	var list []WebShellConnection
	for rows.Next() {
		var c WebShellConnection
		err := rows.Scan(&c.ID, &c.ProjectID, &c.URL, &c.Password, &c.Type, &c.Method, &c.CmdParam, &c.Remark, &c.Encoding, &c.OS, &c.CreatedAt)
		if err != nil {
			db.logger.Warn("failed to scan WebShell connection row", zap.Error(err))
			continue
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

// GetWebshellConnection retrieves a WebShell connection by ID.
func (db *DB) GetWebshellConnection(id string) (*WebShellConnection, error) {
	query := `
		SELECT id, COALESCE(project_id, '') AS project_id, url, password, type, method, cmd_param, remark,
			COALESCE(encoding, '') AS encoding, COALESCE(os, '') AS os, created_at
		FROM webshell_connections WHERE id = ?
	`
	var c WebShellConnection
	err := db.QueryRow(query, id).Scan(&c.ID, &c.ProjectID, &c.URL, &c.Password, &c.Type, &c.Method, &c.CmdParam, &c.Remark, &c.Encoding, &c.OS, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		db.logger.Error("failed to query WebShell connection", zap.Error(err), zap.String("id", id))
		return nil, err
	}
	return &c, nil
}

// CreateWebshellConnection creates a WebShell connection.
func (db *DB) CreateWebshellConnection(c *WebShellConnection) error {
	query := `
		INSERT INTO webshell_connections (id, project_id, url, password, type, method, cmd_param, remark, encoding, os, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, c.ID, strings.TrimSpace(c.ProjectID), c.URL, c.Password, c.Type, c.Method, c.CmdParam, c.Remark, c.Encoding, c.OS, c.CreatedAt)
	if err != nil {
		db.logger.Error("create WebShell connection failed", zap.Error(err), zap.String("id", c.ID))
		return err
	}
	return nil
}

// UpdateWebshellConnection updates a WebShell connection.
func (db *DB) UpdateWebshellConnection(c *WebShellConnection) error {
	query := `
		UPDATE webshell_connections
		SET project_id = ?, url = ?, password = ?, type = ?, method = ?, cmd_param = ?, remark = ?, encoding = ?, os = ?
		WHERE id = ?
	`
	result, err := db.Exec(query, strings.TrimSpace(c.ProjectID), c.URL, c.Password, c.Type, c.Method, c.CmdParam, c.Remark, c.Encoding, c.OS, c.ID)
	if err != nil {
		db.logger.Error("update WebShell connection failed", zap.Error(err), zap.String("id", c.ID))
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteWebshellConnection deletes a WebShell connection.
func (db *DB) DeleteWebshellConnection(id string) error {
	result, err := db.Exec(`DELETE FROM webshell_connections WHERE id = ?`, id)
	if err != nil {
		db.logger.Error("delete WebShell connection failed", zap.Error(err), zap.String("id", id))
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
