package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ValidProjectFactEdgeTypes lists the allowed edge types for the project facts graph.
var ValidProjectFactEdgeTypes = map[string]struct{}{
	"depends_on":    {},
	"leads_to":      {},
	"enables":       {},
	"exploits":      {},
	"discovered_on": {},
	"contains":      {},
	"part_of":       {},
	"supports":      {},
}

// ProjectFactEdge is a relationship edge in the project facts graph (source → target).
type ProjectFactEdge struct {
	ID                   string    `json:"id"`
	ProjectID            string    `json:"project_id"`
	SourceFactKey        string    `json:"source_fact_key"`
	TargetFactKey        string    `json:"target_fact_key"`
	EdgeType             string    `json:"edge_type"`
	Confidence           string    `json:"confidence"` // confirmed | tentative | deprecated
	SourceConversationID string    `json:"source_conversation_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

// ProjectFactEdgeInput is the input for writing an outgoing edge (source → To).
type ProjectFactEdgeInput struct {
	To         string `json:"to"`
	Type       string `json:"type"`
	Confidence string `json:"confidence,omitempty"`
}

// ProjectFactEdgeFromInput is the input for writing an incoming edge (From → current fact).
type ProjectFactEdgeFromInput struct {
	From       string `json:"from"`
	Type       string `json:"type"`
	Confidence string `json:"confidence,omitempty"`
}

// ProjectFactGraphNode is a node in the graph API.
type ProjectFactGraphNode struct {
	ID         string `json:"id"`
	FactKey    string `json:"fact_key"`
	Category   string `json:"category"`
	Label      string `json:"label"`   // Short graph node label (truncated).
	Summary    string `json:"summary"` // Full summary (used for sidebar and other detail views).
	Confidence string `json:"confidence"`
	Type       string `json:"type"`
	Pinned     bool   `json:"pinned"`
}

// ProjectFactGraphEdge is an edge in the graph API.
type ProjectFactGraphEdge struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	Type       string `json:"type"`
	Confidence string `json:"confidence"`
}

// ProjectFactGraph is the project facts graph.
type ProjectFactGraph struct {
	Nodes []ProjectFactGraphNode `json:"nodes"`
	Edges []ProjectFactGraphEdge `json:"edges"`
}

// ValidateProjectFactEdgeType validates an edge type.
func ValidateProjectFactEdgeType(edgeType string) error {
	edgeType = strings.TrimSpace(strings.ToLower(edgeType))
	if edgeType == "" {
		return fmt.Errorf("edge type cannot be empty")
	}
	if _, ok := ValidProjectFactEdgeTypes[edgeType]; !ok {
		return fmt.Errorf("invalid edge type: %s", edgeType)
	}
	return nil
}

func normalizeEdgeConfidence(confidence string) string {
	confidence = strings.TrimSpace(strings.ToLower(confidence))
	switch confidence {
	case "confirmed", "deprecated":
		return confidence
	default:
		return "tentative"
	}
}

// ListProjectFactEdgesByProject lists all edges for a project.
func (db *DB) ListProjectFactEdgesByProject(projectID string) ([]*ProjectFactEdge, error) {
	rows, err := db.Query(
		`SELECT id, project_id, source_fact_key, target_fact_key, edge_type, confidence,
		        COALESCE(source_conversation_id,''), created_at, updated_at
		   FROM project_fact_edges
		  WHERE project_id = ?
		  ORDER BY created_at ASC, rowid ASC`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjectFactEdges(rows)
}

// ListOutgoingProjectFactEdges lists all outgoing edges for a fact.
func (db *DB) ListOutgoingProjectFactEdges(projectID, sourceFactKey string) ([]*ProjectFactEdge, error) {
	rows, err := db.Query(
		`SELECT id, project_id, source_fact_key, target_fact_key, edge_type, confidence,
		        COALESCE(source_conversation_id,''), created_at, updated_at
		   FROM project_fact_edges
		  WHERE project_id = ? AND source_fact_key = ?
		  ORDER BY created_at ASC, rowid ASC`,
		projectID, sourceFactKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjectFactEdges(rows)
}

// ListIncomingProjectFactEdges lists all incoming edges for a fact.
func (db *DB) ListIncomingProjectFactEdges(projectID, targetFactKey string) ([]*ProjectFactEdge, error) {
	rows, err := db.Query(
		`SELECT id, project_id, source_fact_key, target_fact_key, edge_type, confidence,
		        COALESCE(source_conversation_id,''), created_at, updated_at
		   FROM project_fact_edges
		  WHERE project_id = ? AND target_fact_key = ?
		  ORDER BY created_at ASC, rowid ASC`,
		projectID, targetFactKey,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProjectFactEdges(rows)
}

// ReplaceOutgoingProjectFactEdges replaces all outgoing edges for a fact (not called when links are omitted).
func (db *DB) ReplaceOutgoingProjectFactEdges(projectID, sourceFactKey, sourceConversationID string, inputs []ProjectFactEdgeInput) error {
	sourceFactKey = strings.TrimSpace(sourceFactKey)
	if sourceFactKey == "" {
		return fmt.Errorf("source_fact_key cannot be empty")
	}
	if _, err := db.Exec(
		`DELETE FROM project_fact_edges WHERE project_id = ? AND source_fact_key = ?`,
		projectID, sourceFactKey,
	); err != nil {
		return fmt.Errorf("failed to clear old edges: %w", err)
	}
	for _, in := range inputs {
		target := strings.TrimSpace(in.To)
		if target == "" {
			continue
		}
		if err := ValidateFactKey(target); err != nil {
			return fmt.Errorf("invalid target fact_key (%s): %w", target, err)
		}
		if target == sourceFactKey {
			return fmt.Errorf("edge cannot point to itself: %s", sourceFactKey)
		}
		if err := ValidateProjectFactEdgeType(in.Type); err != nil {
			return err
		}
		edge := &ProjectFactEdge{
			ID:                   uuid.New().String(),
			ProjectID:            projectID,
			SourceFactKey:        sourceFactKey,
			TargetFactKey:        target,
			EdgeType:             strings.ToLower(strings.TrimSpace(in.Type)),
			Confidence:           normalizeEdgeConfidence(in.Confidence),
			SourceConversationID: sourceConversationID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		if err := db.insertProjectFactEdge(edge); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceIncomingProjectFactEdges replaces all incoming edges for a fact (From is the source fact_key).
func (db *DB) ReplaceIncomingProjectFactEdges(projectID, targetFactKey string, inputs []ProjectFactEdgeFromInput) error {
	targetFactKey = strings.TrimSpace(targetFactKey)
	if targetFactKey == "" {
		return fmt.Errorf("target_fact_key cannot be empty")
	}
	if _, err := db.Exec(
		`DELETE FROM project_fact_edges WHERE project_id = ? AND target_fact_key = ?`,
		projectID, targetFactKey,
	); err != nil {
		return fmt.Errorf("failed to clear old incoming edges: %w", err)
	}
	for _, in := range inputs {
		source := strings.TrimSpace(in.From)
		if source == "" {
			continue
		}
		if err := ValidateFactKey(source); err != nil {
			return fmt.Errorf("invalid source fact_key (%s): %w", source, err)
		}
		if source == targetFactKey {
			return fmt.Errorf("edge cannot point to itself: %s", targetFactKey)
		}
		if err := ValidateProjectFactEdgeType(in.Type); err != nil {
			return err
		}
		sourceConversationID := ""
		if srcFact, err := db.GetProjectFactByKey(projectID, source); err == nil && srcFact != nil {
			sourceConversationID = srcFact.SourceConversationID
		}
		edge := &ProjectFactEdge{
			ID:                   uuid.New().String(),
			ProjectID:            projectID,
			SourceFactKey:        source,
			TargetFactKey:        targetFactKey,
			EdgeType:             strings.ToLower(strings.TrimSpace(in.Type)),
			Confidence:           normalizeEdgeConfidence(in.Confidence),
			SourceConversationID: sourceConversationID,
			CreatedAt:            time.Now(),
			UpdatedAt:            time.Now(),
		}
		if err := db.insertProjectFactEdge(edge); err != nil {
			return err
		}
	}
	return nil
}

// GetProjectFactEdge retrieves an edge by ID.
func (db *DB) GetProjectFactEdge(edgeID string) (*ProjectFactEdge, error) {
	var e ProjectFactEdge
	var createdAt, updatedAt string
	err := db.QueryRow(
		`SELECT id, project_id, source_fact_key, target_fact_key, edge_type, confidence,
		        COALESCE(source_conversation_id,''), created_at, updated_at
		   FROM project_fact_edges WHERE id = ?`, edgeID,
	).Scan(&e.ID, &e.ProjectID, &e.SourceFactKey, &e.TargetFactKey, &e.EdgeType, &e.Confidence,
		&e.SourceConversationID, &createdAt, &updatedAt)
	if err != nil {
		return nil, fmt.Errorf("edge not found")
	}
	e.CreatedAt = parseDBTime(createdAt)
	e.UpdatedAt = parseDBTime(updatedAt)
	return &e, nil
}

// AddProjectFactEdge adds a single edge (updates confidence if it already exists).
func (db *DB) AddProjectFactEdge(projectID string, in ProjectFactEdgeInput, sourceFactKey, sourceConversationID string) (*ProjectFactEdge, error) {
	sourceFactKey = strings.TrimSpace(sourceFactKey)
	target := strings.TrimSpace(in.To)
	if sourceFactKey == "" || target == "" {
		return nil, fmt.Errorf("source and target are required")
	}
	if sourceFactKey == target {
		return nil, fmt.Errorf("edge cannot point to itself")
	}
	if err := ValidateProjectFactEdgeType(in.Type); err != nil {
		return nil, err
	}
	if err := ValidateFactKey(target); err != nil {
		return nil, err
	}
	now := time.Now()
	e := &ProjectFactEdge{
		ID:                   uuid.New().String(),
		ProjectID:            projectID,
		SourceFactKey:        sourceFactKey,
		TargetFactKey:        target,
		EdgeType:             strings.ToLower(strings.TrimSpace(in.Type)),
		Confidence:           normalizeEdgeConfidence(in.Confidence),
		SourceConversationID: sourceConversationID,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	_, err := db.Exec(
		`INSERT INTO project_fact_edges (
			id, project_id, source_fact_key, target_fact_key, edge_type, confidence,
			source_conversation_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id, source_fact_key, target_fact_key, edge_type)
		DO UPDATE SET confidence = excluded.confidence, updated_at = excluded.updated_at`,
		e.ID, e.ProjectID, e.SourceFactKey, e.TargetFactKey, e.EdgeType, e.Confidence,
		nullIfEmpty(e.SourceConversationID), e.CreatedAt, e.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add edge: %w", err)
	}
	// Return the latest persisted state.
	rows, err := db.Query(
		`SELECT id, project_id, source_fact_key, target_fact_key, edge_type, confidence,
		        COALESCE(source_conversation_id,''), created_at, updated_at
		   FROM project_fact_edges
		  WHERE project_id = ? AND source_fact_key = ? AND target_fact_key = ? AND edge_type = ?`,
		projectID, sourceFactKey, target, e.EdgeType,
	)
	if err != nil {
		return e, nil
	}
	defer rows.Close()
	list, err := scanProjectFactEdges(rows)
	if err != nil || len(list) == 0 {
		return e, nil
	}
	return list[0], nil
}

// DeleteProjectFactEdge deletes a single edge.
func (db *DB) DeleteProjectFactEdge(edgeID string) error {
	res, err := db.Exec(`DELETE FROM project_fact_edges WHERE id = ?`, edgeID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("edge not found")
	}
	return nil
}

func (db *DB) insertProjectFactEdge(e *ProjectFactEdge) error {
	_, err := db.Exec(
		`INSERT INTO project_fact_edges (
			id, project_id, source_fact_key, target_fact_key, edge_type, confidence,
			source_conversation_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.ProjectID, e.SourceFactKey, e.TargetFactKey, e.EdgeType, e.Confidence,
		nullIfEmpty(e.SourceConversationID), e.CreatedAt, e.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to write edge: %w", err)
	}
	return nil
}

// RenameProjectFactKeyEdges syncs edge references when a fact key is renamed.
func (db *DB) RenameProjectFactKeyEdges(projectID, oldKey, newKey string) error {
	oldKey = strings.TrimSpace(oldKey)
	newKey = strings.TrimSpace(newKey)
	if oldKey == "" || newKey == "" || oldKey == newKey {
		return nil
	}
	now := time.Now()
	if _, err := db.Exec(
		`UPDATE project_fact_edges SET source_fact_key = ?, updated_at = ?
		  WHERE project_id = ? AND source_fact_key = ?`,
		newKey, now, projectID, oldKey,
	); err != nil {
		return err
	}
	_, err := db.Exec(
		`UPDATE project_fact_edges SET target_fact_key = ?, updated_at = ?
		  WHERE project_id = ? AND target_fact_key = ?`,
		newKey, now, projectID, oldKey,
	)
	return err
}

// DeleteProjectFactEdgesForKey deletes all edges associated with a fact_key.
func (db *DB) DeleteProjectFactEdgesForKey(projectID, factKey string) error {
	_, err := db.Exec(
		`DELETE FROM project_fact_edges
		  WHERE project_id = ? AND (source_fact_key = ? OR target_fact_key = ?)`,
		projectID, factKey, factKey,
	)
	return err
}

// DeprecateProjectFactEdgesForKey marks all edges associated with a fact_key as deprecated.
func (db *DB) DeprecateProjectFactEdgesForKey(projectID, factKey string) error {
	now := time.Now()
	_, err := db.Exec(
		`UPDATE project_fact_edges SET confidence = 'deprecated', updated_at = ?
		  WHERE project_id = ? AND (source_fact_key = ? OR target_fact_key = ?)
		    AND confidence != 'deprecated'`,
		now, projectID, factKey, factKey,
	)
	return err
}

func scanProjectFactEdges(rows *sql.Rows) ([]*ProjectFactEdge, error) {
	var out []*ProjectFactEdge
	for rows.Next() {
		var e ProjectFactEdge
		var createdAt, updatedAt string
		if err := rows.Scan(
			&e.ID, &e.ProjectID, &e.SourceFactKey, &e.TargetFactKey, &e.EdgeType, &e.Confidence,
			&e.SourceConversationID, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		e.CreatedAt = parseDBTime(createdAt)
		e.UpdatedAt = parseDBTime(updatedAt)
		out = append(out, &e)
	}
	return out, rows.Err()
}
