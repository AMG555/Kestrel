package doctor

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"go.uber.org/zap"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/processguard"
	"kestrel/web"
)

// CheckStatus indicates severity level of a check.
type CheckStatus string

const (
	StatusOK   CheckStatus = "OK"
	StatusWarn CheckStatus = "WARN"
	StatusFail CheckStatus = "FAIL"
)

// CheckResult represents a single diagnostic check.
type CheckResult struct {
	Name        string      `json:"name"`
	Status      CheckStatus `json:"status"`
	Description string      `json:"description"`
	Details     string      `json:"details,omitempty"`
	Remediation string      `json:"remediation,omitempty"`
}

// Report contains all diagnostic check results.
type Report struct {
	Checks      []CheckResult `json:"checks"`
	HasErrors   bool          `json:"has_errors"`
	HasWarnings bool          `json:"has_warnings"`
	Timestamp   time.Time     `json:"timestamp"`
}

// RunDiagnostics executes system health checks against environment and configuration.
func RunDiagnostics(ctx context.Context, cfg *config.Config) *Report {
	rep := &Report{
		Timestamp: time.Now(),
	}

	// 1. Runtime environment
	rep.addCheck(checkRuntime())

	// 2. Embedded Web Assets
	rep.addCheck(checkWebAssets())

	// 3. SQLite Database Access
	rep.addCheck(checkDatabase(cfg))

	// 4. Server Port Availability
	rep.addCheck(checkPort(cfg))

	// 5. Process Isolation Backend
	rep.addCheck(checkProcessIsolation(ctx))

	// 6. Security & Session Secret
	rep.addCheck(checkSecurity(cfg))

	// 7. Pentesting Toolchain
	rep.addCheck(checkToolchain())

	return rep
}

func (r *Report) addCheck(res CheckResult) {
	r.Checks = append(r.Checks, res)
	if res.Status == StatusFail {
		r.HasErrors = true
	} else if res.Status == StatusWarn {
		r.HasWarnings = true
	}
}

func checkRuntime() CheckResult {
	return CheckResult{
		Name:        "Runtime Environment",
		Status:      StatusOK,
		Description: "Go runtime and host architecture",
		Details:     fmt.Sprintf("%s %s/%s (%d CPUs)", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU()),
	}
}

func checkWebAssets() CheckResult {
	_, subFS, err := web.ServerFS()
	if err != nil {
		return CheckResult{
			Name:        "Embedded Web UI",
			Status:      StatusFail,
			Description: "Single-binary embedded SPA assets",
			Details:     fmt.Sprintf("Failed to load embedded filesystem: %v", err),
			Remediation: "Ensure web/dist is built and embedded via go:embed in web/embed.go",
		}
	}

	f, err := subFS.Open("index.html")
	if err != nil {
		return CheckResult{
			Name:        "Embedded Web UI",
			Status:      StatusFail,
			Description: "Single-binary embedded SPA assets",
			Details:     "web/dist/index.html is missing in embedded binary",
			Remediation: "Run 'npm run build' inside web/ and recompile binary",
		}
	}
	defer f.Close()

	stat, _ := f.Stat()
	size := int64(0)
	if stat != nil {
		size = stat.Size()
	}

	return CheckResult{
		Name:        "Embedded Web UI",
		Status:      StatusOK,
		Description: "Single-binary embedded SPA assets",
		Details:     fmt.Sprintf("index.html present (%d bytes embedded)", size),
	}
}

func checkDatabase(cfg *config.Config) CheckResult {
	dbPath := "data/conversations.db"
	if cfg != nil && strings.TrimSpace(cfg.Database.Path) != "" {
		dbPath = strings.TrimSpace(cfg.Database.Path)
	}

	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return CheckResult{
			Name:        "Database Directory",
			Status:      StatusFail,
			Description: "SQLite data storage path",
			Details:     fmt.Sprintf("Cannot create directory %s: %v", dir, err),
			Remediation: "Check disk permissions and ensure process has write access to path",
		}
	}

	// Verify writable directory
	testFile := filepath.Join(dir, fmt.Sprintf(".doctor_test_%d", time.Now().UnixNano()))
	if err := os.WriteFile(testFile, []byte("ok"), 0600); err != nil {
		return CheckResult{
			Name:        "Database Permissions",
			Status:      StatusFail,
			Description: "SQLite storage write permission",
			Details:     fmt.Sprintf("Failed writing to directory %s: %v", dir, err),
			Remediation: "Grant write permissions to directory " + dir,
		}
	}
	_ = os.Remove(testFile)

	// Test SQLite open and table check
	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		remediation := "Check file permissions or SQLite journal/lock status"
		if strings.Contains(err.Error(), "CGO_ENABLED=0") {
			remediation = "Recompile Kestrel with CGO_ENABLED=1 to enable SQLite database engine"
		}
		return CheckResult{
			Name:        "Database Initialization",
			Status:      StatusFail,
			Description: "SQLite database connectivity",
			Details:     fmt.Sprintf("Failed opening database at %s: %v", dbPath, err),
			Remediation: remediation,
		}
	}
	defer db.Close()

	return CheckResult{
		Name:        "Database Connectivity",
		Status:      StatusOK,
		Description: "SQLite database storage",
		Details:     fmt.Sprintf("Active and writable at %s", dbPath),
	}
}

func checkPort(cfg *config.Config) CheckResult {
	host := "0.0.0.0"
	port := 8080
	if cfg != nil {
		if cfg.Server.Host != "" {
			host = cfg.Server.Host
		}
		if cfg.Server.Port > 0 {
			port = cfg.Server.Port
		}
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return CheckResult{
			Name:        "Server Port",
			Status:      StatusWarn,
			Description: fmt.Sprintf("TCP Port %d availability", port),
			Details:     fmt.Sprintf("Address %s is currently in use: %v", addr, err),
			Remediation: "If Kestrel is already running, this is expected; otherwise choose another port in config.yaml",
		}
	}
	_ = l.Close()

	return CheckResult{
		Name:        "Server Port",
		Status:      StatusOK,
		Description: fmt.Sprintf("TCP Port %d availability", port),
		Details:     fmt.Sprintf("Available to bind on %s", addr),
	}
}

func checkProcessIsolation(ctx context.Context) CheckResult {
	checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	backend, err := processguard.Check(checkCtx)
	if err != nil {
		return CheckResult{
			Name:        "Process Isolation",
			Status:      StatusWarn,
			Description: "OS-level task confinement",
			Details:     fmt.Sprintf("Kernel containment check failed: %v", err),
			Remediation: "On Linux, enable cgroup v2; on Windows ensure process containment is permitted",
		}
	}

	return CheckResult{
		Name:        "Process Isolation",
		Status:      StatusOK,
		Description: "OS-level task confinement",
		Details:     fmt.Sprintf("Enforced via %s backend", backend),
	}
}

func checkSecurity(cfg *config.Config) CheckResult {
	jwtSecret := ""
	if cfg != nil {
		jwtSecret = strings.TrimSpace(cfg.Auth.JWTSecret)
	}

	if jwtSecret == "" {
		return CheckResult{
			Name:        "Session Secret",
			Status:      StatusWarn,
			Description: "RFC 7519 JWT signing key",
			Details:     "No auth.jwt_secret configured in config.yaml (using ephemeral fallback)",
			Remediation: "Set 'auth.jwt_secret' in config.yaml or set $JWT_SECRET environment variable",
		}
	}

	if len(jwtSecret) < 32 {
		return CheckResult{
			Name:        "Session Secret",
			Status:      StatusWarn,
			Description: "RFC 7519 JWT signing key",
			Details:     fmt.Sprintf("Configured JWT secret is short (%d characters; minimum 32 recommended)", len(jwtSecret)),
			Remediation: "Generate a 256-bit cryptographically secure string for auth.jwt_secret",
		}
	}

	return CheckResult{
		Name:        "Session Secret",
		Status:      StatusOK,
		Description: "RFC 7519 JWT signing key",
		Details:     "Configured with strong secret key",
	}
}

func checkToolchain() CheckResult {
	tools := []string{"nmap", "masscan", "sqlmap", "nuclei", "nikto", "curl"}
	var found []string
	var missing []string

	for _, tool := range tools {
		if path, err := exec.LookPath(tool); err == nil {
			found = append(found, filepath.Base(path))
		} else {
			missing = append(missing, tool)
		}
	}

	if len(missing) == len(tools) {
		return CheckResult{
			Name:        "External Pentest Tools",
			Status:      StatusWarn,
			Description: "System PATH penetration testing utilities",
			Details:     "No external tools detected in PATH; agent will use native Go probes",
			Remediation: "Optional: install nmap, sqlmap, nuclei in PATH to extend agent tool capabilities",
		}
	}

	details := fmt.Sprintf("Found: %s", strings.Join(found, ", "))
	if len(missing) > 0 {
		details += fmt.Sprintf(" (Missing: %s)", strings.Join(missing, ", "))
	}

	return CheckResult{
		Name:        "External Pentest Tools",
		Status:      StatusOK,
		Description: "System PATH penetration testing utilities",
		Details:     details,
	}
}

// PrintReport writes human-readable diagnostic status to w.
func PrintReport(w io.Writer, rep *Report) {
	fmt.Fprintln(w, "=== Kestrel Diagnostic Doctor ===")
	fmt.Fprintf(w, "Timestamp: %s\n\n", rep.Timestamp.Format(time.RFC1123))

	for _, check := range rep.Checks {
		tag := "[ OK ]"
		switch check.Status {
		case StatusWarn:
			tag = "[WARN]"
		case StatusFail:
			tag = "[FAIL]"
		}

		fmt.Fprintf(w, "%-6s  %-24s : %s\n", tag, check.Name, check.Details)
		if check.Remediation != "" && check.Status != StatusOK {
			fmt.Fprintf(w, "        -> Remediation: %s\n", check.Remediation)
		}
	}

	fmt.Fprintln(w, "-----------------------------------------------------------------")
	if rep.HasErrors {
		fmt.Fprintln(w, "Result: FAIL - One or more critical checks failed.")
	} else if rep.HasWarnings {
		fmt.Fprintln(w, "Result: PASS (with warnings) - System is ready to run.")
	} else {
		fmt.Fprintln(w, "Result: PASS - All system checks passed cleanly.")
	}
	fmt.Fprintln(w, "")
}
