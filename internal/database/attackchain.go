package database

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// AttackChainNode is a node in the attack chain.
type AttackChainNode struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"` // tool, vulnerability, target, exploit
	Label           string                 `json:"label"`
	ToolExecutionID string                 `json:"tool_execution_id,omitempty"`
	Metadata        map[string]interface{} `json:"metadata"`
	RiskScore       int                    `json:"risk_score"`
}

// AttackChainEdge is an edge in the attack chain.
type AttackChainEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"` // leads_to, exploits, enables, depends_on
	Weight int    `json:"weight"`
}

// SaveAttackChainNode saves an attack chain node.
func (db *DB) SaveAttackChainNode(conversationID, nodeID, nodeType, nodeName, toolExecutionID, metadata string, riskScore int) error {
	var toolExecID sql.NullString
	if toolExecutionID != "" {
		toolExecID = sql.NullString{String: toolExecutionID, Valid: true}
	}

	var metadataJSON sql.NullString
	if metadata != "" {
		metadataJSON = sql.NullString{String: metadata, Valid: true}
	}

	query := `
		INSERT OR REPLACE INTO attack_chain_nodes 
		(id, conversation_id, node_type, node_name, tool_execution_id, metadata, risk_score, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`

	_, err := db.Exec(query, nodeID, conversationID, nodeType, nodeName, toolExecID, metadataJSON, riskScore)
	if err != nil {
		db.logger.Error("failed to save attack chain node", zap.Error(err), zap.String("nodeId", nodeID))
		return err
	}

	return nil
}

// SaveAttackChainEdge saves an attack chain edge.
func (db *DB) SaveAttackChainEdge(conversationID, edgeID, sourceNodeID, targetNodeID, edgeType string, weight int) error {
	query := `
		INSERT OR REPLACE INTO attack_chain_edges 
		(id, conversation_id, source_node_id, target_node_id, edge_type, weight, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`

	_, err := db.Exec(query, edgeID, conversationID, sourceNodeID, targetNodeID, edgeType, weight)
	if err != nil {
		db.logger.Error("failed to save attack chain edge", zap.Error(err), zap.String("edgeId", edgeID))
		return err
	}

	return nil
}

// LoadAttackChainNodes loads attack chain nodes for a conversation.
func (db *DB) LoadAttackChainNodes(conversationID string) ([]AttackChainNode, error) {
	query := `
		SELECT id, node_type, node_name, tool_execution_id, metadata, risk_score
		FROM attack_chain_nodes
		WHERE conversation_id = ?
		ORDER BY created_at ASC, rowid ASC
	`

	rows, err := db.Query(query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query attack chain nodes: %w", err)
	}
	defer rows.Close()

	var nodes []AttackChainNode
	for rows.Next() {
		var node AttackChainNode
		var toolExecID sql.NullString
		var metadataJSON sql.NullString

		err := rows.Scan(&node.ID, &node.Type, &node.Label, &toolExecID, &metadataJSON, &node.RiskScore)
		if err != nil {
			db.logger.Warn("failed to scan attack chain node", zap.Error(err))
			continue
		}

		if toolExecID.Valid {
			node.ToolExecutionID = toolExecID.String
		}

		if metadataJSON.Valid && metadataJSON.String != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &node.Metadata); err != nil {
				db.logger.Warn("failed to parse node metadata", zap.Error(err))
				node.Metadata = make(map[string]interface{})
			}
		} else {
			node.Metadata = make(map[string]interface{})
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// LoadAttackChainEdges loads attack chain edges for a conversation.
func (db *DB) LoadAttackChainEdges(conversationID string) ([]AttackChainEdge, error) {
	query := `
		SELECT id, source_node_id, target_node_id, edge_type, weight
		FROM attack_chain_edges
		WHERE conversation_id = ?
		ORDER BY created_at ASC, rowid ASC
	`

	rows, err := db.Query(query, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to query attack chain edges: %w", err)
	}
	defer rows.Close()

	var edges []AttackChainEdge
	for rows.Next() {
		var edge AttackChainEdge

		err := rows.Scan(&edge.ID, &edge.Source, &edge.Target, &edge.Type, &edge.Weight)
		if err != nil {
			db.logger.Warn("failed to scan attack chain edge", zap.Error(err))
			continue
		}

		edges = append(edges, edge)
	}

	return edges, nil
}

// DeleteAttackChain deletes all attack chain data for a conversation.
func (db *DB) DeleteAttackChain(conversationID string) error {
	// Delete edges first (due to foreign key constraints).
	_, err := db.Exec("DELETE FROM attack_chain_edges WHERE conversation_id = ?", conversationID)
	if err != nil {
		db.logger.Warn("failed to delete attack chain edges", zap.Error(err))
	}

	// Then delete nodes.
	_, err = db.Exec("DELETE FROM attack_chain_nodes WHERE conversation_id = ?", conversationID)
	if err != nil {
		db.logger.Error("failed to delete attack chain nodes", zap.Error(err), zap.String("conversationId", conversationID))
		return err
	}

	return nil
}
