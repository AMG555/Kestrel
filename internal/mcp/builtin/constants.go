package builtin

// 内置tool name常量
// 所有代码中使用内置tool name的地方都应该使用这些常量，而不yes硬编码string
const (
	// Vulnerability managementtool
	ToolRecordVulnerability = "record_vulnerability"
	ToolListVulnerabilities = "list_vulnerabilities"
	ToolGetVulnerability    = "get_vulnerability"

	// asset managementtool
	ToolCreateAsset       = "create_asset"
	ToolGetAsset          = "get_asset"
	ToolQueryAssets       = "query_assets"
	ToolUpdateAsset       = "update_asset"
	ToolDeleteAsset       = "delete_asset"
	ToolCompleteAssetScan = "complete_asset_scan"

	// project黑板（事实）tool
	ToolUpsertProjectFact    = "upsert_project_fact"
	ToolGetProjectFact       = "get_project_fact"
	ToolListProjectFacts     = "list_project_facts"
	ToolSearchProjectFacts   = "search_project_facts"
	ToolDeprecateProjectFact = "deprecate_project_fact"
	ToolRestoreProjectFact   = "restore_project_fact"

	// 知识库tool
	ToolListKnowledgeRiskTypes = "list_knowledge_risk_types"
	ToolSearchKnowledgeBase    = "search_knowledge_base"

	// 视觉analyze（本地图片 → VL model → 文本summary）
	ToolAnalyzeImage = "analyze_image"

	// 长耗时tool execution控制（后台 execution 查询/等待/cancelled）
	ToolGetToolExecution    = "get_tool_execution"
	ToolWaitToolExecution   = "wait_tool_execution"
	ToolCancelToolExecution = "cancel_tool_execution"

	// WebShell assistanttool（AI 在 WebShell 管理 - AI assistant 中使用）
	ToolWebshellExec      = "webshell_exec"
	ToolWebshellFileList  = "webshell_file_list"
	ToolWebshellFileRead  = "webshell_file_read"
	ToolWebshellFileWrite = "webshell_file_write"

	// WebShell 连接管理tool（用于通过 MCP 管理 webshell 连接）
	ToolManageWebshellList   = "manage_webshell_list"
	ToolManageWebshellAdd    = "manage_webshell_add"
	ToolManageWebshellUpdate = "manage_webshell_update"
	ToolManageWebshellDelete = "manage_webshell_delete"
	ToolManageWebshellTest   = "manage_webshell_test"

	// 批量taskqueue（与 Web 端批量task一致，供modelcreate/启停/查询queue）
	ToolBatchTaskList            = "batch_task_list"
	ToolBatchTaskGet             = "batch_task_get"
	ToolBatchTaskCreate          = "batch_task_create"
	ToolBatchTaskStart           = "batch_task_start"
	ToolBatchTaskRerun           = "batch_task_rerun"
	ToolBatchTaskPause           = "batch_task_pause"
	ToolBatchTaskDelete          = "batch_task_delete"
	ToolBatchTaskUpdateMetadata  = "batch_task_update_metadata"
	ToolBatchTaskUpdateSchedule  = "batch_task_update_schedule"
	ToolBatchTaskScheduleEnabled = "batch_task_schedule_enabled"
	ToolBatchTaskAdd             = "batch_task_add_task"
	ToolBatchTaskUpdate          = "batch_task_update_task"
	ToolBatchTaskRemove          = "batch_task_remove_task"

	// C2 tool集（合并同类项，8 个统一tool）
	ToolC2Listener   = "c2_listener"    // Listener管理（create/start/stop/list/get/update/delete）
	ToolC2Session    = "c2_session"     // Session管理（list/get/set_sleep/kill/delete）
	ToolC2Task       = "c2_task"        // Task下发（统一 task_type 参数）
	ToolC2TaskManage = "c2_task_manage" // Task管理（get_result/wait/list/cancel）
	ToolC2Payload    = "c2_payload"     // Payload 生成（oneliner/build）
	ToolC2Event      = "c2_event"       // event查询
	ToolC2Profile    = "c2_profile"     // Malleable Profile 管理（list/get/create/update/delete）
	ToolC2File       = "c2_file"        // file管理（list/get_result）
)

// IsBuiltinTool checktool nameyesnoyes内置tool
func IsBuiltinTool(toolName string) bool {
	switch toolName {
	case ToolRecordVulnerability,
		ToolListVulnerabilities,
		ToolGetVulnerability,
		ToolCreateAsset,
		ToolGetAsset,
		ToolQueryAssets,
		ToolUpdateAsset,
		ToolDeleteAsset,
		ToolCompleteAssetScan,
		ToolUpsertProjectFact,
		ToolGetProjectFact,
		ToolListProjectFacts,
		ToolSearchProjectFacts,
		ToolDeprecateProjectFact,
		ToolRestoreProjectFact,
		ToolListKnowledgeRiskTypes,
		ToolSearchKnowledgeBase,
		ToolAnalyzeImage,
		ToolGetToolExecution,
		ToolWaitToolExecution,
		ToolCancelToolExecution,
		ToolWebshellExec,
		ToolWebshellFileList,
		ToolWebshellFileRead,
		ToolWebshellFileWrite,
		ToolManageWebshellList,
		ToolManageWebshellAdd,
		ToolManageWebshellUpdate,
		ToolManageWebshellDelete,
		ToolManageWebshellTest,
		ToolBatchTaskList,
		ToolBatchTaskGet,
		ToolBatchTaskCreate,
		ToolBatchTaskStart,
		ToolBatchTaskRerun,
		ToolBatchTaskPause,
		ToolBatchTaskDelete,
		ToolBatchTaskUpdateMetadata,
		ToolBatchTaskUpdateSchedule,
		ToolBatchTaskScheduleEnabled,
		ToolBatchTaskAdd,
		ToolBatchTaskUpdate,
		ToolBatchTaskRemove,
		// C2 tool
		ToolC2Listener,
		ToolC2Session,
		ToolC2Task,
		ToolC2TaskManage,
		ToolC2Payload,
		ToolC2Event,
		ToolC2Profile,
		ToolC2File:
		return true
	default:
		return false
	}
}

// GetAllBuiltinTools back所有内置tool namelist
func GetAllBuiltinTools() []string {
	return []string{
		ToolRecordVulnerability,
		ToolListVulnerabilities,
		ToolGetVulnerability,
		ToolCreateAsset,
		ToolGetAsset,
		ToolQueryAssets,
		ToolUpdateAsset,
		ToolDeleteAsset,
		ToolCompleteAssetScan,
		ToolUpsertProjectFact,
		ToolGetProjectFact,
		ToolListProjectFacts,
		ToolSearchProjectFacts,
		ToolDeprecateProjectFact,
		ToolRestoreProjectFact,
		ToolListKnowledgeRiskTypes,
		ToolSearchKnowledgeBase,
		ToolAnalyzeImage,
		ToolGetToolExecution,
		ToolWaitToolExecution,
		ToolCancelToolExecution,
		ToolWebshellExec,
		ToolWebshellFileList,
		ToolWebshellFileRead,
		ToolWebshellFileWrite,
		ToolManageWebshellList,
		ToolManageWebshellAdd,
		ToolManageWebshellUpdate,
		ToolManageWebshellDelete,
		ToolManageWebshellTest,
		ToolBatchTaskList,
		ToolBatchTaskGet,
		ToolBatchTaskCreate,
		ToolBatchTaskStart,
		ToolBatchTaskRerun,
		ToolBatchTaskPause,
		ToolBatchTaskDelete,
		ToolBatchTaskUpdateMetadata,
		ToolBatchTaskUpdateSchedule,
		ToolBatchTaskScheduleEnabled,
		ToolBatchTaskAdd,
		ToolBatchTaskUpdate,
		ToolBatchTaskRemove,
		// C2 tool
		ToolC2Listener,
		ToolC2Session,
		ToolC2Task,
		ToolC2TaskManage,
		ToolC2Payload,
		ToolC2Event,
		ToolC2Profile,
		ToolC2File,
	}
}
