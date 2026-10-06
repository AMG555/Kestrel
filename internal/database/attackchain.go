package database

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AttackChainNode is a node in the project attack graph.
type AttackChainNode struct {
	ID          string                 `json:"id"`
	ProjectID   string                 `json:"project_id"`
	SessionID   string                 `json:"session_id"`
	NodeType    string                 `json:"node_type"` // recon | exploit | finding | asset | pivot
	NodeName    string                 `json:"node_name"`
	ToolExecID  string                 `json:"tool_exec_id,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	RiskScore   int                    `json:"risk_score"`
	CreatedAt   time.Time              `json:"created_at"`
}

// AttackChainEdge connects two nodes.
type AttackChainEdge struct {
	ID           string    `json:"id"`
	ProjectID    string    `json:"project_id"`
	SourceNodeID string    `json:"source_node_id"`
	TargetNodeID string    `json:"target_node_id"`
	EdgeType     string    `json:"edge_type"`
	Weight       int       `json:"weight"`
	CreatedAt    time.Time `json:"created_at"`
}

// AddAttackChainNode inserts a node into the attack graph.
func (db *DB) AddAttackChainNode(n *AttackChainNode) (*AttackChainNode, error) {
	n.ID = uuid.New().String()
	n.CreatedAt = time.Now().UTC()
	meta, _ := json.Marshal(n.Metadata)
	_, err := db.Exec(`
		INSERT INTO attack_chain_nodes (id,project_id,session_id,node_type,node_name,tool_exec_id,metadata_json,risk_score,created_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		n.ID, n.ProjectID, n.SessionID, n.NodeType, n.NodeName, n.ToolExecID, string(meta), n.RiskScore, n.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("adding attack chain node: %w", err)
	}
	return n, nil
}

// AddAttackChainEdge inserts an edge into the attack graph.
func (db *DB) AddAttackChainEdge(e *AttackChainEdge) (*AttackChainEdge, error) {
	e.ID = uuid.New().String()
	e.CreatedAt = time.Now().UTC()
	if e.Weight == 0 {
		e.Weight = 1
	}
	_, err := db.Exec(`
		INSERT INTO attack_chain_edges (id,project_id,source_node_id,target_node_id,edge_type,weight,created_at)
		VALUES (?,?,?,?,?,?,?)`,
		e.ID, e.ProjectID, e.SourceNodeID, e.TargetNodeID, e.EdgeType, e.Weight, e.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("adding attack chain edge: %w", err)
	}
	return e, nil
}

// GetAttackChain returns all nodes and edges for a project.
func (db *DB) GetAttackChain(projectID string) ([]*AttackChainNode, []*AttackChainEdge, error) {
	nodeRows, err := db.Query(`
		SELECT id,project_id,session_id,node_type,node_name,tool_exec_id,metadata_json,risk_score,created_at
		FROM attack_chain_nodes WHERE project_id=? ORDER BY created_at`, projectID,
	)
	if err != nil {
		return nil, nil, err
	}
	defer nodeRows.Close()

	var nodes []*AttackChainNode
	for nodeRows.Next() {
		n := &AttackChainNode{}
		var meta string
		if err := nodeRows.Scan(&n.ID, &n.ProjectID, &n.SessionID, &n.NodeType, &n.NodeName, &n.ToolExecID, &meta, &n.RiskScore, &n.CreatedAt); err != nil {
			return nil, nil, err
		}
		_ = json.Unmarshal([]byte(meta), &n.Metadata)
		nodes = append(nodes, n)
	}
	if err := nodeRows.Err(); err != nil {
		return nil, nil, err
	}

	edgeRows, err := db.Query(`
		SELECT id,project_id,source_node_id,target_node_id,edge_type,weight,created_at
		FROM attack_chain_edges WHERE project_id=? ORDER BY created_at`, projectID,
	)
	if err != nil {
		return nil, nil, err
	}
	defer edgeRows.Close()

	var edges []*AttackChainEdge
	for edgeRows.Next() {
		e := &AttackChainEdge{}
		if err := edgeRows.Scan(&e.ID, &e.ProjectID, &e.SourceNodeID, &e.TargetNodeID, &e.EdgeType, &e.Weight, &e.CreatedAt); err != nil {
			return nil, nil, err
		}
		edges = append(edges, e)
	}
	return nodes, edges, edgeRows.Err()
}

// DeleteAttackChainNode removes a node (cascades to edges).
func (db *DB) DeleteAttackChainNode(id string) error {
	_, err := db.Exec(`DELETE FROM attack_chain_nodes WHERE id=?`, id)
	return err
}
