package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// WorkflowDefinition represents a stored workflow graph.
type WorkflowDefinition struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	GraphJSON   string    `json:"graph_json"`
	Enabled     bool      `json:"enabled"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateWorkflowDefinition inserts a new workflow definition.
func (db *DB) CreateWorkflowDefinition(name, description, graphJSON, createdBy string) (string, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO workflow_definitions (id,name,description,graph_json,enabled,created_by,created_at,updated_at)
		VALUES (?,?,?,?,1,?,?,?)`,
		id, name, description, graphJSON, createdBy, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("creating workflow definition: %w", err)
	}
	return id, nil
}

// GetWorkflowDefinition retrieves a workflow definition by ID.
func (db *DB) GetWorkflowDefinition(id string) (*WorkflowDefinition, error) {
	var w WorkflowDefinition
	err := db.QueryRow(`
		SELECT id,name,description,graph_json,enabled,created_by,created_at,updated_at
		FROM workflow_definitions WHERE id=?`, id,
	).Scan(&w.ID, &w.Name, &w.Description, &w.GraphJSON, &w.Enabled, &w.CreatedBy, &w.CreatedAt, &w.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("workflow not found")
	}
	return &w, err
}

// UpdateWorkflowDefinition updates mutable fields on a workflow definition.
// Nil pointer arguments mean "do not update that field".
func (db *DB) UpdateWorkflowDefinition(id, name, description string, graphJSON *string, enabled *bool) error {
	now := time.Now().UTC()
	// Fetch current values.
	cur, err := db.GetWorkflowDefinition(id)
	if err != nil {
		return err
	}
	newName := cur.Name
	if name != "" {
		newName = name
	}
	newDesc := cur.Description
	if description != "" {
		newDesc = description
	}
	newGraph := cur.GraphJSON
	if graphJSON != nil {
		newGraph = *graphJSON
	}
	newEnabled := cur.Enabled
	if enabled != nil {
		newEnabled = *enabled
	}
	_, err = db.Exec(`
		UPDATE workflow_definitions SET name=?,description=?,graph_json=?,enabled=?,updated_at=? WHERE id=?`,
		newName, newDesc, newGraph, newEnabled, now, id,
	)
	return err
}

// DeleteWorkflowDefinition removes a workflow definition and all its runs.
func (db *DB) DeleteWorkflowDefinition(id string) error {
	_, err := db.Exec(`DELETE FROM workflow_definitions WHERE id=?`, id)
	return err
}
