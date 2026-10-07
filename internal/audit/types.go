package audit

// Category constants for platform auditing.
const (
	CategoryAuth       = "auth"
	CategorySecurity   = "security"
	CategoryTerminal   = "terminal"
	CategoryWorkflow   = "workflow"
	CategoryTool       = "tool"
	CategoryConfig     = "config"
	CategorySystem     = "system"
	CategoryKnowledge  = "knowledge"
	CategoryProject    = "project"
)

// Action constants.
const (
	ActionLogin          = "login"
	ActionLogout         = "logout"
	ActionChangePassword = "change_password"
	ActionExec           = "exec"
	ActionToolRun        = "tool_run"
	ActionPolicyCheck    = "policy_check"
	ActionConfigUpdate   = "config_update"
	ActionPurge          = "purge"
)

// Result constants.
const (
	ResultSuccess = "success"
	ResultFailure = "failure"
	ResultBlocked = "blocked"
)

// Entry describes one platform audit record.
type Entry struct {
	Level        string                 `json:"level"` // info, warn, error
	Category     string                 `json:"category"`
	Action       string                 `json:"action"`
	Result       string                 `json:"result"` // success | failure | blocked
	ActorID      string                 `json:"actor_id,omitempty"`
	ActorName    string                 `json:"actor_name,omitempty"`
	SessionHint  string                 `json:"session_hint,omitempty"`
	ResourceType string                 `json:"resource_type,omitempty"`
	ResourceID   string                 `json:"resource_id,omitempty"`
	Message      string                 `json:"message"`
	Detail       map[string]interface{} `json:"detail,omitempty"`
	ClientIP     string                 `json:"client_ip,omitempty"`
	UserAgent    string                 `json:"user_agent,omitempty"`
}
