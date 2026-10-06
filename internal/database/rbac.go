package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Role represents an RBAC role with scoped tool permissions.
type Role struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	IsSystem     bool      `json:"is_system"`
	Permissions  []string  `json:"permissions"`
	AllowedTools []string  `json:"allowed_tools"`
	HITLMode     string    `json:"hitl_mode"` // auto | require_approval
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateRole inserts a new role.
func (db *DB) CreateRole(name, description string, isSystem bool, permissions, allowedTools []string, hitlMode string) (*Role, error) {
	if hitlMode == "" {
		hitlMode = "auto"
	}
	permJSON, _ := json.Marshal(permissions)
	toolsJSON, _ := json.Marshal(allowedTools)
	now := time.Now().UTC()
	r := &Role{
		ID:           uuid.New().String(),
		Name:         name,
		Description:  description,
		IsSystem:     isSystem,
		Permissions:  permissions,
		AllowedTools: allowedTools,
		HITLMode:     hitlMode,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := db.Exec(`
		INSERT INTO roles (id,name,description,is_system,permissions_json,allowed_tools_json,hitl_mode,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Name, r.Description, boolToInt(r.IsSystem),
		string(permJSON), string(toolsJSON), r.HITLMode,
		r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("creating role: %w", err)
	}
	return r, nil
}

// GetRoleByID retrieves a role by ID.
func (db *DB) GetRoleByID(id string) (*Role, error) {
	return db.scanRole(db.QueryRow(
		`SELECT id,name,description,is_system,permissions_json,allowed_tools_json,hitl_mode,created_at,updated_at
		 FROM roles WHERE id=?`, id,
	))
}

// GetRoleByName retrieves a role by name.
func (db *DB) GetRoleByName(name string) (*Role, error) {
	return db.scanRole(db.QueryRow(
		`SELECT id,name,description,is_system,permissions_json,allowed_tools_json,hitl_mode,created_at,updated_at
		 FROM roles WHERE name=?`, name,
	))
}

// ListRoles returns all roles.
func (db *DB) ListRoles() ([]*Role, error) {
	rows, err := db.Query(
		`SELECT id,name,description,is_system,permissions_json,allowed_tools_json,hitl_mode,created_at,updated_at
		 FROM roles ORDER BY is_system DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*Role
	for rows.Next() {
		r, err := db.scanRoleRow(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

// UpdateRole updates a role's mutable fields.
func (db *DB) UpdateRole(id, description string, permissions, allowedTools []string, hitlMode string) error {
	permJSON, _ := json.Marshal(permissions)
	toolsJSON, _ := json.Marshal(allowedTools)
	_, err := db.Exec(
		`UPDATE roles SET description=?, permissions_json=?, allowed_tools_json=?, hitl_mode=?, updated_at=?
		 WHERE id=?`,
		description, string(permJSON), string(toolsJSON), hitlMode, time.Now().UTC(), id,
	)
	return err
}

// DeleteRole removes a non-system role.
func (db *DB) DeleteRole(id string) error {
	_, err := db.Exec(`DELETE FROM roles WHERE id=? AND is_system=0`, id)
	return err
}

// AssignRole assigns a role to a user.
func (db *DB) AssignRole(userID, roleID, assignedBy string) error {
	_, err := db.Exec(`
		INSERT OR REPLACE INTO user_roles (user_id,role_id,assigned_at,assigned_by)
		VALUES (?,?,?,?)`,
		userID, roleID, time.Now().UTC(), assignedBy,
	)
	return err
}

// RevokeRole removes a role assignment from a user.
func (db *DB) RevokeRole(userID, roleID string) error {
	_, err := db.Exec(`DELETE FROM user_roles WHERE user_id=? AND role_id=?`, userID, roleID)
	return err
}

// GetUserRoles returns all roles for a user.
func (db *DB) GetUserRoles(userID string) ([]*Role, error) {
	rows, err := db.Query(`
		SELECT r.id,r.name,r.description,r.is_system,r.permissions_json,r.allowed_tools_json,r.hitl_mode,r.created_at,r.updated_at
		FROM roles r
		JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id=?
		ORDER BY r.name`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*Role
	for rows.Next() {
		r, err := db.scanRoleRow(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

// UserHasPermission checks if a user holds a specific permission via any role.
func (db *DB) UserHasPermission(userID, permission string) (bool, error) {
	roles, err := db.GetUserRoles(userID)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		for _, p := range r.Permissions {
			if p == permission || p == "*" {
				return true, nil
			}
		}
	}
	return false, nil
}

// UserCanUseTool checks if a user is allowed to invoke a specific tool via any role.
func (db *DB) UserCanUseTool(userID, toolName string) (bool, error) {
	roles, err := db.GetUserRoles(userID)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		for _, t := range r.AllowedTools {
			if t == toolName || t == "*" {
				return true, nil
			}
		}
	}
	return false, nil
}

func (db *DB) scanRole(row *sql.Row) (*Role, error) {
	r := &Role{}
	var isSystem int
	var permJSON, toolsJSON string
	err := row.Scan(&r.ID, &r.Name, &r.Description, &isSystem, &permJSON, &toolsJSON, &r.HITLMode, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.IsSystem = isSystem == 1
	_ = json.Unmarshal([]byte(permJSON), &r.Permissions)
	_ = json.Unmarshal([]byte(toolsJSON), &r.AllowedTools)
	return r, nil
}

func (db *DB) scanRoleRow(rows *sql.Rows) (*Role, error) {
	r := &Role{}
	var isSystem int
	var permJSON, toolsJSON string
	err := rows.Scan(&r.ID, &r.Name, &r.Description, &isSystem, &permJSON, &toolsJSON, &r.HITLMode, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	r.IsSystem = isSystem == 1
	_ = json.Unmarshal([]byte(permJSON), &r.Permissions)
	_ = json.Unmarshal([]byte(toolsJSON), &r.AllowedTools)
	return r, nil
}
