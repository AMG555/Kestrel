// Package c2 implements the Kestrel built-in C2 (Command & Control) framework.
//
// Design overview:
//   - Manager is the unified entry point, instantiated by internal/app and injected into all components that need to control C2
//     (HTTP handler, MCP tool, HITL bridge, attack chain recorder, etc.).
//   - Listener is an abstract interface, with concrete implementations for tcp_reverse / http_beacon / https_beacon / websocket
//     and other transports, all created through the listener.Registry factory.
//   - Task scheduling uses a hybrid of database (c2_tasks table) + in-memory event bus (EventBus):
//     * status changes and history use SQLite for persistence and restart recovery;
//     * high-frequency real-time notifications (e.g. new task results) are pushed via EventBus to SSE/WS subscribers, avoiding polling.
//   - Crypto layer uses fixed AES-256-GCM; each Listener has its own 32-byte key; keys are only held server-side
//     and injected into the implant at compile time; the event stream must never export plaintext keys.
package c2

import (
	"errors"
	"strings"
	"time"
)

// ListenerType is the listener type, consistent with the c2_listeners.type field
type ListenerType string

const (
	ListenerTypeTCPReverse   ListenerType = "tcp_reverse"
	ListenerTypeHTTPBeacon   ListenerType = "http_beacon"
	ListenerTypeHTTPSBeacon  ListenerType = "https_beacon"
	ListenerTypeWebSocket    ListenerType = "websocket"
)

// AllListenerTypes lists all supported listener types, for validation and frontend enumeration
func AllListenerTypes() []ListenerType {
	return []ListenerType{
		ListenerTypeTCPReverse,
		ListenerTypeHTTPBeacon,
		ListenerTypeHTTPSBeacon,
		ListenerTypeWebSocket,
	}
}

// IsValidListenerType validates whether a frontend/MCP input is a valid type
func IsValidListenerType(t string) bool {
	t = strings.ToLower(strings.TrimSpace(t))
	for _, lt := range AllListenerTypes() {
		if string(lt) == t {
			return true
		}
	}
	return false
}

// SessionStatus is consistent with the c2_sessions.status field
type SessionStatus string

const (
	SessionActive   SessionStatus = "active"
	SessionSleeping SessionStatus = "sleeping"
	SessionDead     SessionStatus = "dead"
	SessionKilled   SessionStatus = "killed"
)

// TaskStatus is consistent with the c2_tasks.status field
type TaskStatus string

const (
	TaskQueued    TaskStatus = "queued"
	TaskSent      TaskStatus = "sent"
	TaskRunning   TaskStatus = "running"
	TaskSuccess   TaskStatus = "success"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

// TaskType is the task type (negotiated with the beacon side to avoid hardcoded strings)
type TaskType string

const (
	// generic tasks
	TaskTypeExec       TaskType = "exec"        // execute arbitrary command (shell -c)
	TaskTypeShell      TaskType = "shell"       // interactive command (preserves cwd)
	TaskTypePwd        TaskType = "pwd"         // current directory
	TaskTypeCd         TaskType = "cd"          // change directory
	TaskTypeLs         TaskType = "ls"          // list directory
	TaskTypePs         TaskType = "ps"          // list processes
	TaskTypeKillProc   TaskType = "kill_proc"   // kill process
	TaskTypeUpload     TaskType = "upload"      // push file to target
	TaskTypeDownload   TaskType = "download"    // pull file back to local
	TaskTypeScreenshot TaskType = "screenshot"  // take screenshot
	TaskTypeSleep      TaskType = "sleep"       // adjust heartbeat interval
	TaskTypeExit       TaskType = "exit"        // tell implant to exit (does not self-delete binary)
	TaskTypeSelfDelete TaskType = "self_delete" // exit + self-delete binary (persistence cleanup)
	// advanced tasks
	TaskTypePortFwd      TaskType = "port_fwd"
	TaskTypeSocksStart   TaskType = "socks_start"
	TaskTypeSocksStop    TaskType = "socks_stop"
	TaskTypeLoadAssembly TaskType = "load_assembly"
	TaskTypePersist      TaskType = "persist"
)

// AllTaskTypes lists all task_type values, for tool schema enum listing
func AllTaskTypes() []TaskType {
	return []TaskType{
		TaskTypeExec, TaskTypeShell,
		TaskTypePwd, TaskTypeCd, TaskTypeLs, TaskTypePs, TaskTypeKillProc,
		TaskTypeUpload, TaskTypeDownload, TaskTypeScreenshot,
		TaskTypeSleep, TaskTypeExit, TaskTypeSelfDelete,
		TaskTypePortFwd, TaskTypeSocksStart, TaskTypeSocksStop, TaskTypeLoadAssembly,
		TaskTypePersist,
	}
}

// IsDangerousTaskType marks task types that require HITL secondary confirmation;
// corresponds to the existing tool_whitelist concept in internal/handler/hitl.go: non-whitelisted → requires approval.
func IsDangerousTaskType(t TaskType) bool {
	switch t {
	case TaskTypeKillProc, TaskTypeUpload, TaskTypeSelfDelete,
		TaskTypePortFwd, TaskTypeSocksStart, TaskTypeLoadAssembly, TaskTypePersist:
		return true
	}
	return false
}

// ListenerConfig is the decoded listener runtime config (from c2_listeners.config_json)
type ListenerConfig struct {
	// HTTP/HTTPS Beacon common fields
	BeaconCheckInPath string `json:"beacon_check_in_path,omitempty"` // default "/check_in"
	BeaconTasksPath   string `json:"beacon_tasks_path,omitempty"`    // default "/tasks"
	BeaconResultPath  string `json:"beacon_result_path,omitempty"`   // default "/result"
	BeaconUploadPath  string `json:"beacon_upload_path,omitempty"`   // default "/upload"
	BeaconFilePath    string `json:"beacon_file_path,omitempty"`     // default "/file/"
	// HTTPS-only
	TLSCertPath string `json:"tls_cert_path,omitempty"`
	TLSKeyPath  string `json:"tls_key_path,omitempty"`
	TLSAutoSelfSign bool `json:"tls_auto_self_sign,omitempty"` // true: auto-generate self-signed cert when no cert is found
	// Client default parameters (written to c2_sessions initial value; beacon can override at check-in)
	DefaultSleep  int `json:"default_sleep,omitempty"`  // seconds, default 5
	DefaultJitter int `json:"default_jitter,omitempty"` // 0-100, default 0
	// OPSEC: optional command blacklist (regex)
	CommandDenyRegex []string `json:"command_deny_regex,omitempty"`
	// Task concurrency limit (max tasks dispatched simultaneously per session, 0 means unlimited)
	MaxConcurrentTasks int `json:"max_concurrent_tasks,omitempty"`
	// CallbackHost is the callback hostname used by the implant/payload (optional); separated from bind_host for NAT/ECS scenarios
	CallbackHost string `json:"callback_host,omitempty"`
	// AllowLegacyShell: when true, tcp_reverse allows unencrypted classic bash/nc reverse shells to register sessions (default false; strongly not recommended for public deployment)
	AllowLegacyShell bool `json:"allow_legacy_shell,omitempty"`
}

// ApplyDefaults fills unset fields with default values; the caller is responsible for serializing the new values on persistence.
func (c *ListenerConfig) ApplyDefaults() {
	if strings.TrimSpace(c.BeaconCheckInPath) == "" {
		c.BeaconCheckInPath = "/check_in"
	}
	if strings.TrimSpace(c.BeaconTasksPath) == "" {
		c.BeaconTasksPath = "/tasks"
	}
	if strings.TrimSpace(c.BeaconResultPath) == "" {
		c.BeaconResultPath = "/result"
	}
	if strings.TrimSpace(c.BeaconUploadPath) == "" {
		c.BeaconUploadPath = "/upload"
	}
	if strings.TrimSpace(c.BeaconFilePath) == "" {
		c.BeaconFilePath = "/file/"
	}
	if c.DefaultSleep <= 0 {
		c.DefaultSleep = 5
	}
	if c.DefaultJitter < 0 {
		c.DefaultJitter = 0
	}
	if c.DefaultJitter > 100 {
		c.DefaultJitter = 100
	}
}

// ImplantCheckInRequest is the registration/heartbeat request body sent from a beacon to the server (decrypted plaintext).
type ImplantCheckInRequest struct {
	ImplantUUID  string                 `json:"uuid"`
	Hostname     string                 `json:"hostname"`
	Username     string                 `json:"username"`
	OS           string                 `json:"os"`
	Arch         string                 `json:"arch"`
	PID          int                    `json:"pid"`
	ProcessName  string                 `json:"process_name"`
	IsAdmin      bool                   `json:"is_admin"`
	InternalIP   string                 `json:"internal_ip"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	SleepSeconds int                    `json:"sleep_seconds"`
	JitterPercent int                   `json:"jitter_percent"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ImplantCheckInResponse is the server acknowledgement.
type ImplantCheckInResponse struct {
	SessionID    string `json:"session_id"`
	NextSleep    int    `json:"next_sleep"`
	NextJitter   int    `json:"next_jitter"`
	HasTasks     bool   `json:"has_tasks"`
	ServerTime   int64  `json:"server_time"`
}

// TaskEnvelope is the task dispatch carrier from the server to a beacon.
type TaskEnvelope struct {
	TaskID   string                 `json:"task_id"`
	TaskType string                 `json:"task_type"`
	Payload  map[string]interface{} `json:"payload"`
}

// TaskResultReport is the task result reported back from a beacon to the server.
type TaskResultReport struct {
	TaskID     string `json:"task_id"`
	Success    bool   `json:"success"`
	Output     string `json:"output,omitempty"`
	OutputB64  string `json:"output_b64,omitempty"` // raw console bytes (base64) to avoid JSON breaking non-UTF-8 output
	Error      string `json:"error,omitempty"`
	ErrorB64   string `json:"error_b64,omitempty"`
	BlobBase64 string `json:"blob_b64,omitempty"` // e.g. screenshot binary
	BlobSuffix string `json:"blob_suffix,omitempty"` // e.g. ".png"
	StartedAt  int64  `json:"started_at"`
	EndedAt    int64  `json:"ended_at"`
}

// CommonError is the unified error type for the C2 module, enabling the handler layer to map HTTP status codes.
type CommonError struct {
	Code    string
	Message string
	HTTP    int
}

func (e *CommonError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Sentinel errors for use with errors.Is comparisons.
var (
	ErrListenerNotFound = &CommonError{Code: "listener_not_found", Message: "listener not found", HTTP: 404}
	ErrSessionNotFound  = &CommonError{Code: "session_not_found", Message: "session not found", HTTP: 404}
	ErrTaskNotFound     = &CommonError{Code: "task_not_found", Message: "task not found", HTTP: 404}
	ErrProfileNotFound  = &CommonError{Code: "profile_not_found", Message: "profile not found", HTTP: 404}
	ErrInvalidInput     = &CommonError{Code: "invalid_input", Message: "invalid parameters", HTTP: 400}
	ErrAuthFailed       = &CommonError{Code: "auth_failed", Message: "authentication failed", HTTP: 401}
	ErrPortInUse        = &CommonError{Code: "port_in_use", Message: "port is already in use", HTTP: 409}
	ErrListenerRunning  = &CommonError{Code: "listener_running", Message: "listener is already running", HTTP: 409}
	ErrListenerStopped  = &CommonError{Code: "listener_stopped", Message: "listener is not running", HTTP: 409}
	ErrUnsupportedType  = &CommonError{Code: "unsupported_type", Message: "unsupported listener type", HTTP: 400}
)

// SafeBindPort validates the port range.
func SafeBindPort(port int) error {
	if port < 1 || port > 65535 {
		return errors.New("port must be in 1..65535")
	}
	return nil
}

// NowUnixMillis returns the current time as a unified Unix millisecond timestamp.
func NowUnixMillis() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}
