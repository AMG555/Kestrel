package app

import (
	"context"
	"fmt"
	"strings"

	"kestrel/internal/agent"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/mcp"
	"kestrel/internal/mcp/builtin"
	"kestrel/internal/project"

	"go.uber.org/zap"
)

func projectIDFromConversation(db *database.DB, ctx context.Context) (string, error) {
	convID := agent.ConversationIDFromContext(ctx)
	if convID == "" {
		return "", fmt.Errorf("cannot resolve the current conversation; use the project facts tool within a conversation context")
	}
	pid, err := db.GetConversationProjectID(convID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(pid) == "" {
		return "", fmt.Errorf("the current conversation is not bound to a project; please select a project in the conversation or create a conversation with a project")
	}
	return pid, nil
}

func textResult(msg string, isErr bool) *mcp.ToolResult {
	return &mcp.ToolResult{
		Content: []mcp.Content{{Type: "text", Text: msg}},
		IsError: isErr,
	}
}

// registerProjectFactTools registers the project blackboard MCP tools.
func registerProjectFactTools(mcpServer *mcp.Server, db *database.DB, cfg *config.Config, logger *zap.Logger) {
	if db == nil || cfg == nil || !cfg.Project.Enabled {
		if logger != nil {
			logger.Info("project blackboard tool not registered (not enabled)")
		}
		return
	}

	upsertTool := mcp.Tool{
		Name: builtin.ToolUpsertProjectFact,
		Description: "Write or update a project blackboard fact, used to persist reproducible context across sessions (not a formal vulnerability entry; use record_vulnerability for deliverable vulnerabilities). " +
			"Record as you pentest: call immediately after confirming each new insight (port/entry/credentials/exploitable point), overwrite-update with the same fact_key, do not wait until the session ends. " +
			"Do not write conclusions only: summary must contain what+where+how to validate; body must contain attack chain/request-response/commands and other reproduction details. " +
			"Discovery entries: recommended fact_key format is finding|chain|exploit|poc/<slug>, category corresponds to finding|chain|exploit|poc, body written per attack chain template. " +
			"Environment entries: use target|auth|infra|business/<slug>. Same fact_key overwrites. Requires the current conversation to be bound to a project.",
		ShortDescription: "Write/update project facts (including attack chain body)",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"fact_key": map[string]interface{}{
					"type":        "string",
					"description": "Project-unique key: target/primary_domain, finding/sqli-login, exploit/upload-rce, etc.",
				},
				"category": map[string]interface{}{
					"type":        "string",
					"description": "target | auth | infra | business | finding | chain | exploit | poc | note",
					"enum":        []string{"target", "auth", "infra", "business", "finding", "chain", "exploit", "poc", "note"},
				},
				"summary": map[string]interface{}{
					"type":        "string",
					"description": "One-line index: conclusion + location + trigger/validation key points (do not write vague summaries like \"XSS exists\")",
				},
				"body": map[string]interface{}{
					"type": "string",
					"description": "Complete reproducible details (returned by get_project_fact only): must include attack chain steps, raw HTTP/commands, response observations, evidence and associations." +
						"Required for first-time Discovery/exploitation entries; environment entries should include source evidence. Attack chain entries can follow the template sections: conclusion, target and entry point, attack chain, Exploit/POC, key evidence, associations, Remarks. " +
						"When updating an existing fact_key, if body is omitted or left empty, the existing body in the database is kept (you can change only the summary).",
				},
				"confidence": map[string]interface{}{
					"type":        "string",
					"description": "confirmed | tentative | deprecated",
					"enum":        []string{"confirmed", "tentative", "deprecated"},
				},
				"pinned": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to appear first in the blackboard index",
				},
				"related_vulnerability_id": map[string]interface{}{
					"type":        "string",
					"description": "Optional: associated vulnerability record ID",
				},
				"links": map[string]interface{}{
					"type":        "array",
					"description": "Optional: relationship edges (from → current fact). Findings require at least 1 {from:target/*, type:discovered_on}; to record an exploit on a finding use {from:exploit/*, type:exploits}. Omit to preserve existing edges; pass [] to clear all relationship edges.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"from": map[string]interface{}{
								"type":        "string",
								"description": "Source fact_key: stored as from → current fact",
							},
							"type": map[string]interface{}{
								"type":        "string",
								"description": "depends_on | leads_to | enables | exploits | discovered_on | contains | part_of | supports",
							},
							"confidence": map[string]interface{}{
								"type":        "string",
								"description": "confirmed | tentative | deprecated",
							},
						},
						"required": []string{"from", "type"},
					},
				},
			},
			"required": []string{"fact_key", "summary"},
		},
	}

	mcpServer.RegisterTool(upsertTool, func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		projectID, err := projectIDFromConversation(db, ctx)
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		factKey, _ := args["fact_key"].(string)
		summary, _ := args["summary"].(string)
		if strings.TrimSpace(factKey) == "" || strings.TrimSpace(summary) == "" {
			return textResult("error: fact_key and summary are required", true), nil
		}
		if len([]rune(summary)) > cfg.Project.FactSummaryMaxRunesEffective() {
			return textResult(fmt.Sprintf("error: summary too long (max %d characters)", cfg.Project.FactSummaryMaxRunesEffective()), true), nil
		}
		f := &database.ProjectFact{
			ProjectID:              projectID,
			FactKey:                factKey,
			Category:               strArg(args, "category"),
			Summary:                summary,
			Body:                   strArg(args, "body"),
			Confidence:             strArg(args, "confidence"),
			Pinned:                 boolArg(args, "pinned"),
			RelatedVulnerabilityID: strArg(args, "related_vulnerability_id"),
		}
		if convID := agent.ConversationIDFromContext(ctx); convID != "" {
			f.SourceConversationID = convID
		}
		created, err := db.UpsertProjectFact(f)
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		if _, hasLinks := args["links"]; hasLinks {
			linkInputs, err := project.ParseFactLinkInputs(args["links"])
			if err != nil {
				return textResult("error: "+err.Error(), true), nil
			}
			convID := agent.ConversationIDFromContext(ctx)
			if err := project.PersistFactLinksFromParsed(db, projectID, created.FactKey, convID, linkInputs, true); err != nil {
				return textResult("error: failed to save relation edge: "+err.Error(), true), nil
			}
			created, _ = db.GetProjectFactByKey(projectID, created.FactKey)
		} else if parsed := project.ParseLinksFromBody(created.Body); len(parsed) > 0 {
			if err := project.PersistFactIncomingLinks(db, projectID, created.FactKey, parsed, true); err != nil {
				return textResult("error: failed to parse edges from body: "+err.Error(), true), nil
			}
			created, _ = db.GetProjectFactByKey(projectID, created.FactKey)
		}
		msg := fmt.Sprintf("fact saved.\nfact_key: %s\nid: %s\nconfidence: %s", created.FactKey, created.ID, created.Confidence)
		if in, _ := db.ListIncomingProjectFactEdges(projectID, created.FactKey); len(in) > 0 {
			msg += "\nrelation edges: " + project.FormatFactLinksText(in)
		}
		if warn := project.SparseBodyWarningIfNeeded(f.Category, f.FactKey, f.Body); warn != "" {
			msg += warn
		}
		return textResult(msg, false), nil
	})

	getTool := mcp.Tool{
		Name:             builtin.ToolGetProjectFact,
		Description:      "Retrieve the full body and metadata of a project fact by fact_key. When the summary is insufficient, this tool must be called; do not fabricate details.",
		ShortDescription: "Get fact details by key",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"fact_key": map[string]interface{}{"type": "string", "description": "fact key"},
			},
			"required": []string{"fact_key"},
		},
	}
	mcpServer.RegisterTool(getTool, func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		projectID, err := projectIDFromConversation(db, ctx)
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		key := strings.TrimSpace(strArg(args, "fact_key"))
		if key == "" {
			return textResult("error: fact_key is required", true), nil
		}
		f, err := db.GetProjectFactByKey(projectID, key)
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		msg := fmt.Sprintf("fact_key: %s\ncategory: %s\nconfidence: %s\nsummary: %s\nupdated_at: %s",
			f.FactKey, f.Category, f.Confidence, f.Summary, f.UpdatedAt.Format("2006-01-02 15:04:05"))
		if f.RelatedVulnerabilityID != "" {
			msg += fmt.Sprintf("\nrelated_vulnerability_id: %s", f.RelatedVulnerabilityID)
		}
		if f.SourceConversationID != "" {
			msg += fmt.Sprintf("\nsource_conversation_id: %s", f.SourceConversationID)
		}
		if in, _ := db.ListIncomingProjectFactEdges(projectID, f.FactKey); len(in) > 0 {
			msg += "\nrelation edges (from → this fact):\n"
			for _, e := range in {
				msg += fmt.Sprintf("- %s ← %s (%s)\n", e.EdgeType, e.SourceFactKey, e.Confidence)
			}
		}
		if out, _ := db.ListOutgoingProjectFactEdges(projectID, f.FactKey); len(out) > 0 {
			msg += "pointing to other facts:\n"
			for _, e := range out {
				msg += fmt.Sprintf("- %s → %s (%s)\n", e.EdgeType, e.TargetFactKey, e.Confidence)
			}
		}
		msg += "\n\n--- body ---\n" + f.Body
		if warn := project.SparseBodyWarningIfNeeded(f.Category, f.FactKey, f.Body); warn != "" {
			msg += warn
		}
		return textResult(msg, false), nil
	})

	listTool := mcp.Tool{
		Name:             builtin.ToolListProjectFacts,
		Description:      "List facts for the current project (paginated).",
		ShortDescription: "List project facts",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"category":   map[string]interface{}{"type": "string"},
				"confidence": map[string]interface{}{"type": "string"},
				"limit":      map[string]interface{}{"type": "integer"},
				"offset":     map[string]interface{}{"type": "integer"},
			},
		},
	}
	mcpServer.RegisterTool(listTool, func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		projectID, err := projectIDFromConversation(db, ctx)
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		limit := intArg(args, "limit", 50)
		offset := intArg(args, "offset", 0)
		filter := database.ProjectFactListFilter{
			Category:   strArg(args, "category"),
			Confidence: strArg(args, "confidence"),
		}
		list, err := db.ListProjectFacts(projectID, filter, limit, offset)
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("total %d item(s) (limit=%d offset=%d):\n", len(list), limit, offset))
		for _, f := range list {
			b.WriteString(fmt.Sprintf("- [%s] %s — %s (%s)\n", f.FactKey, f.Category, f.Summary, f.Confidence))
		}
		return textResult(b.String(), false), nil
	})

	searchTool := mcp.Tool{
		Name:             builtin.ToolSearchProjectFacts,
		Description:      "Search project facts by keyword (summary/body/fact_key).",
		ShortDescription: "Search project facts",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query":  map[string]interface{}{"type": "string"},
				"limit":  map[string]interface{}{"type": "integer"},
				"offset": map[string]interface{}{"type": "integer"},
			},
			"required": []string{"query"},
		},
	}
	mcpServer.RegisterTool(searchTool, func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		projectID, err := projectIDFromConversation(db, ctx)
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		q := strings.TrimSpace(strArg(args, "query"))
		if q == "" {
			return textResult("error: query is required", true), nil
		}
		list, err := db.ListProjectFacts(projectID, database.ProjectFactListFilter{Search: q}, intArg(args, "limit", 30), intArg(args, "offset", 0))
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("search \"%s\" matched %d item(s):\n", q, len(list)))
		for _, f := range list {
			b.WriteString(fmt.Sprintf("- [%s] %s — %s\n", f.FactKey, f.Category, f.Summary))
		}
		return textResult(b.String(), false), nil
	})

	deprecateTool := mcp.Tool{
		Name:             builtin.ToolDeprecateProjectFact,
		Description:      "Mark a fact as deprecated, excluding it from the blackboard index.",
		ShortDescription: "Deprecate project facts",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"fact_key": map[string]interface{}{"type": "string"},
			},
			"required": []string{"fact_key"},
		},
	}
	mcpServer.RegisterTool(deprecateTool, func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		projectID, err := projectIDFromConversation(db, ctx)
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		key := strings.TrimSpace(strArg(args, "fact_key"))
		if err := db.DeprecateProjectFact(projectID, key); err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		return textResult("fact marked as deprecated: "+key, false), nil
	})

	restoreTool := mcp.Tool{
		Name:             builtin.ToolRestoreProjectFact,
		Description:      "Restore a deprecated fact to tentative or confirmed status, re-including it in the blackboard index.",
		ShortDescription: "Restore deprecated project facts",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"fact_key": map[string]interface{}{"type": "string"},
				"confidence": map[string]interface{}{
					"type":        "string",
					"description": "Confidence after resume: tentative (default) or confirmed",
					"enum":        []string{"tentative", "confirmed"},
				},
			},
			"required": []string{"fact_key"},
		},
	}
	mcpServer.RegisterTool(restoreTool, func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		projectID, err := projectIDFromConversation(db, ctx)
		if err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		key := strings.TrimSpace(strArg(args, "fact_key"))
		if key == "" {
			return textResult("error: fact_key is required", true), nil
		}
		conf := strArg(args, "confidence")
		if err := db.RestoreProjectFact(projectID, key, conf); err != nil {
			return textResult("error: "+err.Error(), true), nil
		}
		if conf == "" {
			conf = "tentative"
		}
		return textResult(fmt.Sprintf("fact restored to %s: %s", conf, key), false), nil
	})

	if logger != nil {
		logger.Debug("project blackboard MCP tools registered successfully")
	}
}

func strArg(args map[string]interface{}, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

func boolArg(args map[string]interface{}, key string) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return false
}

func intArg(args map[string]interface{}, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	default:
		return def
	}
}
