package handler

// apiDocI18n provides x-i18n-* extension keys for OpenAPI documentation, used by the frontend for API doc internationalization.
// The frontend translates via apiDocs.tags.* / apiDocs.summary.* / apiDocs.response.*.

var apiDocI18nTagToKey = map[string]string{
	"Authentication": "auth", "Conversation Management": "conversationManagement", "Conversation Interaction": "conversationInteraction",
	"Batch Tasks": "batchTasks", "Vulnerability management": "vulnerabilityManagement",
	"Role Management": "roleManagement", "Skills Management": "skillsManagement", "Monitoring": "monitoring",
	"config management": "configManagement", "external MCP management": "externalMCPManagement", "attack chain": "attackChain",
	"Knowledge Base": "knowledgeBase", "MCP": "mcp",
	"FOFA Recon": "fofaRecon", "Terminal": "terminal", "WebShell Management": "webshellManagement",
	"conversation attachments": "chatUploads", "bot integration": "robotIntegration", "multi-agent markdown": "markdownAgents",
	"Project Management": "projectManagement", "Asset Management": "assetManagement",
}

var apiDocI18nSummaryToKey = map[string]string{
	"User login": "login", "user logout": "logout", "change password": "changePassword", "validateToken": "validateToken",
	"create conversation": "createConversation", "list conversations": "listConversations", "viewconversationdetails": "getConversationDetail",
	"updateconversation": "updateConversation", "delete conversation": "deleteConversation", "get conversation result": "getConversationResult",
	"Send message and get AI reply (non-streaming)": "sendMessageNonStream", "Send message and get AI reply (streaming)": "sendMessageStream",
	"cancel task": "cancelTask", "list running tasks": "listRunningTasks", "list completed tasks": "listCompletedTasks",
	"create batch task queue": "createBatchQueue", "list batch task queues": "listBatchQueues", "get batch task queue": "getBatchQueue",
	"delete batch task queue": "deleteBatchQueue", "start batch task queue": "startBatchQueue", "pause batch task queue": "pauseBatchQueue",
	"add task to queue": "addTaskToQueue", "SQL Injectionscan": "sqlInjectionScan", "portscan": "portScan",
	"update batch task": "updateBatchTask", "delete batch task": "deleteBatchTask",
	"list vulnerabilities": "listVulnerabilities", "create vulnerability": "createVulnerability", "get vulnerability statistics": "getVulnerabilityStats",
	"get vulnerability": "getVulnerability", "update vulnerability": "updateVulnerability", "delete vulnerability": "deleteVulnerability",
	"list roles": "listRoles", "create role": "createRole", "get role": "getRole", "update role": "updateRole", "delete role": "deleteRole",
	"Get available skills list": "getAvailableSkills", "list skills": "listSkills", "createSkill": "createSkill",
	"get skill statistics": "getSkillStats", "clear skill statistics": "clearSkillStats", "get skill": "getSkill",
	"updateSkill": "updateSkill", "deleteSkill": "deleteSkill", "get bound roles": "getBoundRoles",
	"get monitoring info": "getMonitorInfo", "get execution record": "getExecutionRecords", "delete execution record": "deleteExecutionRecord",
	"batch delete execution records": "batchDeleteExecutionRecords", "get statistics info": "getStats",
	"get configuration": "getConfig", "update configuration": "updateConfig", "get tool configuration": "getToolConfig", "apply configuration": "applyConfig",
	"list external MCPs": "listExternalMCP", "get external MCP statistics": "getExternalMCPStats", "get external MCP": "getExternalMCP",
	"add or update external MCP": "addOrUpdateExternalMCP", "stdiopatternconfig": "stdioModeConfig", "SSEpatternconfig": "sseModeConfig",
	"delete external MCP": "deleteExternalMCP", "start external MCP": "startExternalMCP", "stop external MCP": "stopExternalMCP",
	"get attack chain": "getAttackChain", "regenerate attack chain": "regenerateAttackChain",
	"pin/unpin conversation": "pinConversation",
	"get categories":   "getCategories", "list knowledge items": "listKnowledgeItems", "create knowledge item": "createKnowledgeItem",
	"get knowledge item": "getKnowledgeItem", "update knowledge item": "updateKnowledgeItem", "delete knowledge item": "deleteKnowledgeItem",
	"get index status": "getIndexStatus", "build index": "startKnowledgeIndex", "scan knowledge base": "scanKnowledgeBase",
	"search knowledge base": "searchKnowledgeBase", "basic search": "basicSearch", "search by risk type": "searchByRiskType",
	"get retrieval log": "getRetrievalLogs", "delete retrieval log": "deleteRetrievalLog",
	"MCP endpoint": "mcpEndpoint", "list all tools": "listAllTools", "call tool": "invokeTool", "initialize connection": "initConnection",
	"successfulresponse": "successResponse", "errorresponse": "errorResponse",
	// Add missing endpoints.
	"delete conversation round": "deleteConversationTurn", "get cancellation message process details": "getMessageProcessDetails",
	"re-run batch task queue": "rerunBatchQueue", "modify queue metadata": "updateBatchQueueMetadata",
	"modify queue scheduling config": "updateBatchQueueSchedule", "toggle Cron automatic scheduling": "setBatchQueueScheduleEnabled",
	"FOFAsearch": "fofaSearch", "parse natural language to FOFA syntax": "fofaParse",
	"test OpenAI API connection": "testOpenAI",
	"execute terminal command":         "terminalRun", "streaming terminal command execution": "terminalRunStream", "WebSocket terminal": "terminalWS",
	"list WebShell connections": "listWebshellConnections", "create WebShell connection": "createWebshellConnection",
	"update WebShell connection": "updateWebshellConnection", "delete WebShell connection": "deleteWebshellConnection",
	"get connection status": "getWebshellConnectionState", "saveconnection status": "saveWebshellConnectionState",
	"get AI conversation history": "getWebshellAIHistory", "list AI conversations": "listWebshellAIConversations",
	"execute WebShell command": "webshellExec", "WebShell file operations": "webshellFileOp",
	"list attachments": "listChatUploads", "export attachment": "exportChatUploads", "upload attachment": "uploadChatFile", "delete attachment": "deleteChatUpload",
	"download attachment": "downloadChatUpload", "get attachment text content": "getChatUploadContent",
	"write attachment text content": "putChatUploadContent", "create attachment directory": "mkdirChatUpload", "rename attachment": "renameChatUpload",
	"WeCom callback verification": "wecomCallbackVerify", "WeCom message callback": "wecomCallbackMessage",
	"DingTalk message callback": "dingtalkCallback", "Feishu message callback": "larkCallback", "test robot message processing": "testRobot",
	"list Markdown agents": "listMarkdownAgents", "create Markdown agent": "createMarkdownAgent",
	"get Markdown agent details": "getMarkdownAgent", "update Markdown agent": "updateMarkdownAgent", "delete Markdown agent": "deleteMarkdownAgent",
	"list skill pack files": "listSkillPackageFiles", "get skill pack file content": "getSkillPackageFile", "write skill pack file": "putSkillPackageFile",
	"batch get tool names": "batchGetToolNames",
	"get knowledge base statistics":  "getKnowledgeStats",
	"List projects":     "listProjects", "Create project": "createProject", "Get project": "getProject",
	"update project": "updateProject", "delete project": "deleteProject",
	"bulk import assets":        "importAssets",
	"List or get fact by key": "listProjectFacts", "Create/update fact": "upsertProjectFact",
	"Get project facts attack path graph": "getProjectFactGraph", "List all project fact edges": "listProjectFactEdges",
	"Add fact edge": "createProjectFactEdge", "Delete fact edge": "deleteProjectFactEdge",
	"Promote conversation attack chain to project facts graph": "promoteAttackChainToProject",
}

var apiDocI18nResponseDescToKey = map[string]string{
	"retrieved successfully": "getSuccess", "Unauthorized": "unauthorized", "Unauthorized, a valid token is required": "unauthorizedToken",
	"created successfully": "createSuccess", "request parameter error": "badRequest", "conversation not found": "conversationNotFound",
	"conversation not found or result does not exist": "conversationOrResultNotFound", "request parameter error (e.g. task is empty)": "badRequestTaskEmpty",
	"Request parameter error (e.g. invalid config format, missing required fields)": "badRequestConfig",
	"Request parameter error (e.g. empty query)":         "badRequestQueryEmpty", "Method not allowed (only POST requests supported)": "methodNotAllowed",
	"login successful": "loginSuccess", "incorrect password": "invalidPassword", "logout successful": "logoutSuccess",
	"Password changed successfully": "passwordChanged", "Token valid": "tokenValid", "Token invalid or expired": "tokenInvalid",
	"conversation created successfully": "conversationCreated", "server internal error": "internalError", "update successful": "updateSuccess",
	"deleted successfully": "deleteSuccess", "queue not found": "queueNotFound", "started successfully": "startSuccess",
	"pausesuccessful": "pauseSuccess", "Added successfully": "addSuccess",
	"task not found":   "taskNotFound",
	"cancellation request submitted": "cancelSubmitted", "running task not found": "noRunningTask",
	"Message sent successfully, AI reply returned": "messageSent", "Streaming response (Server-Sent Events)": "streamResponse",
	// Add missing endpoints.response
	"Parameter error or delete failed": "badRequestOrDeleteFailed",
	"Parameter error":      "paramError", "Only completed or cancelled queues can be re-run": "onlyCompletedOrCancelledCanRerun",
	"Parameter error or queue is running": "badRequestOrQueueRunning", "Settings saved successfully": "setSuccess",
	"Search successful": "searchSuccess", "parsed successfully": "parseSuccess", "Test result": "testResult",
	"Execution completed": "executionDone", "SSE event stream": "sseEventStream", "WebSocket connection established": "wsEstablished",
	"file download": "fileDownload", "file not found": "fileNotFound", "Write successful": "writeSuccess",
	"renamed successfully": "renameSuccess", "Validation successful, returns decrypted echostr": "wecomVerifySuccess",
	"Processing successful": "processSuccess", "Agent not found": "agentNotFound", "savesuccessful": "saveSuccess",
	"Operation result": "operationResult", "execution result": "executionResult", "Connection not found": "connectionNotFound",
	"projectlist": "projectList", "projectdetails": "projectDetail",
	"fact list or single item (may include link_counts / outgoing_links)": "projectFactList",
	"successful": "success", "nodes + edges": "factGraphNodesEdges",
	"edge list": "edgeList", "edge created": "edgeCreated",
	"sedimentation result (facts/edges/graph)": "promoteAttackChainResult",
	"Import completed":                    "assetImportCompleted", "Quantity or asset field validation failed": "assetImportValidationFailed",
	"Missing asset:write permission or access denied for specified project": "assetImportForbidden",
	"Import transaction failed": "assetImportTransactionFailed",
}

// enrichSpecWithI18nKeys writes x-i18n-tags and x-i18n-summary on each operation in the spec,
// and x-i18n-description on each response, for frontend internationalization by key.
func enrichSpecWithI18nKeys(spec map[string]interface{}) {
	paths, _ := spec["paths"].(map[string]interface{})
	if paths == nil {
		return
	}
	for _, pathItem := range paths {
		pm, _ := pathItem.(map[string]interface{})
		if pm == nil {
			continue
		}
		for _, method := range []string{"get", "post", "put", "delete", "patch"} {
			opVal, ok := pm[method]
			if !ok {
				continue
			}
			op, _ := opVal.(map[string]interface{})
			if op == nil {
				continue
			}
			// x-i18n-tags: i18n key array corresponding 1-to-1 with tags (tags in spec is []string).
			switch tags := op["tags"].(type) {
			case []string:
				if len(tags) > 0 {
					keys := make([]string, 0, len(tags))
					for _, s := range tags {
						if k := apiDocI18nTagToKey[s]; k != "" {
							keys = append(keys, k)
						} else {
							keys = append(keys, s)
						}
					}
					op["x-i18n-tags"] = keys
				}
			case []interface{}:
				if len(tags) > 0 {
					keys := make([]interface{}, 0, len(tags))
					for _, t := range tags {
						if s, ok := t.(string); ok {
							if k := apiDocI18nTagToKey[s]; k != "" {
								keys = append(keys, k)
							} else {
								keys = append(keys, s)
							}
						}
					}
					if len(keys) > 0 {
						op["x-i18n-tags"] = keys
					}
				}
			}
			// x-i18n-summary
			if summary, _ := op["summary"].(string); summary != "" {
				if k := apiDocI18nSummaryToKey[summary]; k != "" {
					op["x-i18n-summary"] = k
				}
			}
			// responses -> each status -> x-i18n-description
			if respMap, _ := op["responses"].(map[string]interface{}); respMap != nil {
				for _, rv := range respMap {
					if r, _ := rv.(map[string]interface{}); r != nil {
						if desc, _ := r["description"].(string); desc != "" {
							if k := apiDocI18nResponseDescToKey[desc]; k != "" {
								r["x-i18n-description"] = k
							}
						}
					}
				}
			}
		}
	}
}
