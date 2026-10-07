// Package attackchain provides automated DAG construction and promotion of session findings into project attack chains.
package attackchain

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"kestrel/internal/database"
)

// PromoteResult summarizes findings converted into DAG nodes and project facts.
type PromoteResult struct {
	ProjectID    string   `json:"project_id"`
	NodesCreated int      `json:"nodes_created"`
	EdgesCreated int      `json:"edges_created"`
	FactsCreated int      `json:"facts_created"`
	NodeNames    []string `json:"node_names"`
}

// PromoteSessionFindings inspects completed tool executions and vulnerabilities for a session
// and synthesizes them into attack chain DAG nodes, edges, and project facts.
func PromoteSessionFindings(db *database.DB, projectID, sessionID string) (*PromoteResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	projectID = strings.TrimSpace(projectID)
	sessionID = strings.TrimSpace(sessionID)
	if projectID == "" {
		return nil, fmt.Errorf("project_id is required")
	}

	result := &PromoteResult{
		ProjectID: projectID,
		NodeNames: make([]string, 0),
	}

	// 1. Fetch tool executions
	toolExecs, _, err := db.ListToolExecutions(database.ListToolExecutionsParams{
		SessionID: sessionID,
		Limit:     50,
	})
	if err != nil {
		return nil, fmt.Errorf("listing tool executions: %w", err)
	}

	// 2. Fetch project vulnerabilities
	vulns, _, err := db.ListVulnerabilities(database.ListVulnsParams{
		ProjectID: projectID,
		Limit:     50,
	})
	if err != nil {
		vulns = []*database.Vulnerability{}
	}

	var previousNodeID string

	// Create nodes for significant tool executions
	for _, exec := range toolExecs {
		if exec.Status != "completed" {
			continue
		}

		nodeType := "recon"
		riskScore := 20
		toolName := strings.ToLower(exec.ToolName)
		if strings.Contains(toolName, "vuln") || strings.Contains(toolName, "sql") || strings.Contains(toolName, "nuclei") {
			nodeType = "exploit"
			riskScore = 70
		} else if strings.Contains(toolName, "probe") || strings.Contains(toolName, "fingerprint") || strings.Contains(toolName, "port") {
			nodeType = "probe"
			riskScore = 40
		}

		nodeName := fmt.Sprintf("%s (%s)", exec.ToolName, truncate(exec.ArgumentsJSON, 30))
		node := &database.AttackChainNode{
			ProjectID:  projectID,
			SessionID:  sessionID,
			NodeType:   nodeType,
			NodeName:   nodeName,
			ToolExecID: exec.ID,
			Metadata: map[string]interface{}{
				"tool_exec_id": exec.ID,
				"duration_ms":  exec.DurationMs,
				"tool":         exec.ToolName,
			},
			RiskScore: riskScore,
		}

		createdNode, err := db.AddAttackChainNode(node)
		if err == nil {
			result.NodesCreated++
			result.NodeNames = append(result.NodeNames, nodeName)

			if previousNodeID != "" {
				edge := &database.AttackChainEdge{
					ProjectID:    projectID,
					SourceNodeID: previousNodeID,
					TargetNodeID: createdNode.ID,
					EdgeType:     "leads_to",
					Weight:       1,
				}
				if _, err := db.AddAttackChainEdge(edge); err == nil {
					result.EdgesCreated++
				}
			}
			previousNodeID = createdNode.ID
		}
	}

	// Create nodes for linked vulnerabilities
	for _, v := range vulns {
		if v.SessionID != "" && v.SessionID != sessionID {
			continue
		}

		riskScore := 50
		switch strings.ToLower(v.Severity) {
		case "critical":
			riskScore = 95
		case "high":
			riskScore = 80
		case "medium":
			riskScore = 60
		case "low":
			riskScore = 30
		}

		nodeName := fmt.Sprintf("Vuln: %s", v.Title)
		node := &database.AttackChainNode{
			ProjectID: projectID,
			SessionID: sessionID,
			NodeType:  "vulnerability",
			NodeName:  nodeName,
			RiskScore: riskScore,
			Metadata: map[string]interface{}{
				"vuln_id":  v.ID,
				"severity": v.Severity,
			},
		}

		createdNode, err := db.AddAttackChainNode(node)
		if err == nil {
			result.NodesCreated++
			result.NodeNames = append(result.NodeNames, nodeName)

			if previousNodeID != "" {
				edge := &database.AttackChainEdge{
					ProjectID:    projectID,
					SourceNodeID: previousNodeID,
					TargetNodeID: createdNode.ID,
					EdgeType:     "exploited_as",
					Weight:       2,
				}
				if _, err := db.AddAttackChainEdge(edge); err == nil {
					result.EdgesCreated++
				}
			}
			previousNodeID = createdNode.ID
		}

		// Also record fact
		factKey := fmt.Sprintf("vuln.%s", v.ID[:min(8, len(v.ID))])
		_, _ = db.UpsertProjectFact(&database.ProjectFact{
			ID:           uuid.NewString(),
			ProjectID:    projectID,
			FactKey:      factKey,
			Category:     "finding",
			Summary:      v.Title,
			Body:         v.Description,
			Confidence:   "verified",
			SourceSessID: sessionID,
		})
		result.FactsCreated++
	}

	return result, nil
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
