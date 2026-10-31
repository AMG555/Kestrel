package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"kestrel/internal/app"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/demo"
	"kestrel/internal/doctor"
	"kestrel/internal/eval"
	"kestrel/internal/logger"
	"kestrel/internal/processguard"
	"kestrel/internal/report"
	"kestrel/internal/security"
	"kestrel/internal/termout"
	"kestrel/internal/toolguard"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/term"
)

func main() {
	isDoctorSubcommand := false
	isDemoSubcommand := false
	isEvalSubcommand := false
	isAuditCheckpointSubcommand := false
	isAuditVerifySubcommand := false
	var auditArgPath string
	if len(os.Args) > 1 {
		if os.Args[1] == "doctor" {
			isDoctorSubcommand = true
			os.Args = append(os.Args[:1], os.Args[2:]...)
		} else if os.Args[1] == "demo" {
			isDemoSubcommand = true
			os.Args = append(os.Args[:1], os.Args[2:]...)
		} else if os.Args[1] == "eval" {
			isEvalSubcommand = true
			os.Args = append(os.Args[:1], os.Args[2:]...)
		} else if os.Args[1] == "audit" && len(os.Args) > 2 {
			sub := strings.ToLower(strings.TrimSpace(os.Args[2]))
			if sub == "checkpoint" {
				isAuditCheckpointSubcommand = true
				if len(os.Args) > 3 && !strings.HasPrefix(os.Args[3], "-") {
					auditArgPath = os.Args[3]
					os.Args = append(os.Args[:1], os.Args[4:]...)
				} else {
					os.Args = append(os.Args[:1], os.Args[3:]...)
				}
			} else if sub == "verify" {
				isAuditVerifySubcommand = true
				if len(os.Args) > 3 && !strings.HasPrefix(os.Args[3], "-") {
					auditArgPath = os.Args[3]
					os.Args = append(os.Args[:1], os.Args[4:]...)
				} else {
					os.Args = append(os.Args[:1], os.Args[3:]...)
				}
			}
		}
	}
	var configPath = flag.String("config", "config.yaml", "Path to the configuration file")
	var httpsBootstrap = flag.Bool("https", false, "Enable HTTPS for the main site; uses an in-memory self-signed certificate when no cert/key is configured")
	var httpBootstrap = flag.Bool("http", false, "Force plain HTTP for the main site, overriding TLS settings in the configuration file")
	var resetAdminPassword = flag.Bool("reset-admin-password", false, "Interactively reset the built-in admin password and exit")
	checkIsolation := flag.Bool("check-process-isolation", false, "Probe task containment and cleanup, then exit without starting services")
	var exportSarif = flag.Bool("export-sarif", false, "Export vulnerabilities as OASIS SARIF 2.1.0 to stdout or -sarif-out file and exit")
	var sarifProject = flag.String("sarif-project", "", "Filter vulnerabilities for specific project ID in SARIF export")
	var sarifOut = flag.String("sarif-out", "", "Output destination file path for SARIF export (defaults to stdout)")
	var sarifFailOn = flag.String("sarif-fail-on", "", "CI quality gate: exit with code 2 if any finding severity >= threshold (critical, high, medium, low, info)")
	var sarifSeedFixture = flag.Bool("sarif-seed-fixture", false, "Seed database with a test vulnerability fixture before SARIF export")
	var retestVuln = flag.String("retest-vuln", "", "Vulnerability ID to automatically retest using PoC verification (exit 0=resolved/fixed, 1=vulnerable, 2=error)")
	var retestTarget = flag.String("retest-target", "", "Optional target URL override for -retest-vuln")
	var reportShare = flag.String("report-share", "", "Generate a signed read-only share permalink for project ID and exit")
	var shareTTL = flag.Duration("share-ttl", 30*24*time.Hour, "Validity duration for the generated share permalink (e.g. 720h, 30d)")
	var sharePassphrase = flag.String("share-passphrase", "", "Optional passphrase protection for the generated share permalink")
	var shareBaseURL = flag.String("share-base-url", "", "Base URL for share links and badges (default: http(s)://<host>:<port>)")
	var doctorFlag = flag.Bool("doctor", false, "Run diagnostic health checks on runtime, storage, isolation, and tools")
	var demoFlag = flag.Bool("demo", false, "Run in one-command demo mode seeded with OWASP Juice Shop project, admin login, and findings")
	var evalFlag = flag.Bool("eval", false, "Run golden security benchmark evaluation against OWASP targets and exit")
	var scopeFilePath = flag.String("scope-file", "", "Path to engagement scope.yaml file to enforce network-layer egress filtering")
	var scopeExempt = flag.String("scope-exempt", "", "Comma-separated list of additional hosts/domains exempt from scope egress blocking")
	var allowIMDS = flag.Bool("allow-imds", false, "Explicitly permit network egress to cloud instance metadata services (IMDS / 169.254.169.254)")
	var auditCheckpointFlag = flag.String("audit-checkpoint", "", "Create an external signed cryptographic checkpoint receipt of the audit chain to file and exit")
	var auditVerifyFlag = flag.Bool("audit-verify", false, "Verify cryptographic integrity of audit chain and exit (0=valid, 2=broken)")
	var auditAnchorFlag = flag.String("audit-anchor", "", "Optional external checkpoint JSON file to assert against during audit verification")
	flag.Parse()

	// Check bare subcommands from flag.Args()
	for _, arg := range flag.Args() {
		switch strings.ToLower(strings.TrimSpace(arg)) {
		case "doctor":
			isDoctorSubcommand = true
		case "demo":
			isDemoSubcommand = true
		case "eval":
			isEvalSubcommand = true
		}
	}

	// environment variable compatibility (for systemd/docker scenarios where args are not passed)
	if *httpsBootstrap && *httpBootstrap {
		fmt.Fprintln(os.Stderr, "--http and --https cannot be used together")
		os.Exit(2)
	}
	if !*httpsBootstrap && !*httpBootstrap {
		v := strings.TrimSpace(os.Getenv("KESTREL_HTTPS"))
		if v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") {
			*httpsBootstrap = true
		}
	}

	// load config
	cp := strings.TrimSpace(*configPath)
	if cp == "" {
		cp = "config.yaml"
	}
	if strings.HasPrefix(cp, "-") {
		fmt.Fprintf(os.Stderr, "Invalid -config path %q.\nIf HTTPS is also needed, use: ./kestrel --https -config config.yaml (-config must be followed by a yaml file path).\n", cp)
		os.Exit(2)
	}
	localConfig, err := config.EnsureLocalConfig(cp)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	cfg, err := config.Load(cp)
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}
	if localConfig.Created {
		termout.PrintConfigCreated()
	}

	// ensure persistent jwt_secret is configured with 0600 file permissions
	if err := config.EnsureJWTSecret(cp, cfg); err != nil {
		fmt.Printf("Failed to configure JWT secret: %v\n", err)
		return
	}

	if isDoctorSubcommand || *doctorFlag {
		rep := doctor.RunDiagnostics(context.Background(), cfg)
		doctor.PrintReport(os.Stdout, rep)
		if rep.HasErrors {
			os.Exit(1)
		}
		return
	}

	if isDemoSubcommand || *demoFlag {
		*httpBootstrap = true
		if strings.TrimSpace(cfg.Database.Path) == "" || cfg.Database.Path == "data/conversations.db" {
			cfg.Database.Path = "data/demo.db"
		}
	}

	if isEvalSubcommand || *evalFlag {
		rep := eval.RunEvalSuite(context.Background(), "juice-shop-golden")
		eval.PrintEvalReport(os.Stdout, rep)
		if rep.FalseNegatives > 0 || rep.FalsePositives > 0 {
			os.Exit(1)
		}
		return
	}

	// Network egress guard is unconditionally active: blocks IMDS/link-local by default
	var scopePolicy *toolguard.ScopeFilePolicy
	if strings.TrimSpace(*scopeFilePath) != "" {
		var err error
		scopePolicy, err = toolguard.LoadScopeFile(strings.TrimSpace(*scopeFilePath))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Scope file error: %v\n", err)
			os.Exit(2)
		}
	}
	egressGuard := toolguard.NewEgressGuard(scopePolicy)
	if *allowIMDS {
		egressGuard.SetAllowIMDS(true)
	}

	// Auto-exempt LLM endpoints so model inference is never blocked by scope guard
	for _, ch := range cfg.AI.Channels {
		if ch.BaseURL != "" {
			egressGuard.AddExemptHosts(ch.BaseURL)
		}
	}
	if strings.TrimSpace(*scopeExempt) != "" {
		for _, h := range strings.Split(*scopeExempt, ",") {
			egressGuard.AddExemptHosts(h)
		}
	}

	toolguard.SetGlobalEgressGuard(egressGuard)
	if scopePolicy != nil {
		fmt.Printf("[SCOPE GUARD] Enforcing network-layer egress policy from %s (target=%q, in_scope_cidrs=%d, in_scope_domains=%d, out_of_scope_cidrs=%d, fail_closed=%v, allow_imds=%v)\n",
			*scopeFilePath, scopePolicy.TargetName, len(scopePolicy.InScopeCIDRs), len(scopePolicy.InScopeDomains), len(scopePolicy.OutOfScopeCIDRs), scopePolicy.FailClosed, *allowIMDS)
	} else {
		fmt.Printf("[EGRESS GUARD] Enforcing baseline network protection (cloud metadata IMDS blocked, allow_imds=%v)\n", *allowIMDS)
	}

	if isAuditCheckpointSubcommand || *auditCheckpointFlag != "" {
		outFile := auditArgPath
		if outFile == "" {
			outFile = *auditCheckpointFlag
		}
		if err := runAuditCheckpoint(cfg, outFile); err != nil {
			fmt.Fprintf(os.Stderr, "FAILURE: %v\n", err)
			os.Exit(2)
		}
		return
	}

	if isAuditVerifySubcommand || *auditVerifyFlag {
		anchorFile := auditArgPath
		if anchorFile == "" {
			anchorFile = *auditAnchorFlag
		}
		if err := runAuditVerify(cfg, anchorFile); err != nil {
			fmt.Fprintf(os.Stderr, "FAILURE: %v\n", err)
			os.Exit(2)
		}
		return
	}

	if *checkIsolation {
		if err := configureProcessIsolation(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		checkCtx, cancelCheck := context.WithTimeout(context.Background(), 15*time.Second)
		backend, checkErr := processguard.Check(checkCtx)
		cancelCheck()
		if checkErr != nil {
			fmt.Fprintln(os.Stderr, checkErr)
			os.Exit(1)
		}
		fmt.Printf("{\"checked\":true,\"backend\":%q}\n", backend)
		return
	}

	if *resetAdminPassword {
		if err := runResetAdminPassword(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to reset admin password: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *exportSarif || *sarifProject != "" {
		if err := runExportSarif(cfg, *sarifProject, *sarifOut, *sarifFailOn, *sarifSeedFixture); err != nil {
			var qgErr *qualityGateFailedError
			if errors.As(err, &qgErr) {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
			fmt.Fprintf(os.Stderr, "SARIF export error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *retestVuln != "" {
		if err := runRetestVuln(cfg, *retestVuln, *retestTarget); err != nil {
			var retestErr *retestExitError
			if errors.As(err, &retestErr) {
				os.Exit(retestErr.exitCode)
			}
			fmt.Fprintf(os.Stderr, "Retest error: %v\n", err)
			os.Exit(2)
		}
		return
	}

	if *reportShare != "" {
		if err := runReportShare(cfg, *reportShare, *shareTTL, *sharePassphrase, *shareBaseURL); err != nil {
			fmt.Fprintf(os.Stderr, "Report share error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *httpBootstrap {
		config.ApplyPlainHTTPBootstrap(cfg)
	} else if *httpsBootstrap {
		config.ApplyDevHTTPSBootstrap(cfg)
	}

	port := cfg.Server.Port
	if port <= 0 {
		port = 8080
	}
	scheme := "http"
	if config.MainWebUIUsesHTTPS(&cfg.Server) {
		scheme = "https"
	}
	termout.PrintStartupWebUI(termout.StartupWebUIOptions{
		Scheme:       scheme,
		Host:         cfg.Server.Host,
		Port:         port,
		SelfSigned:   scheme == "https" && cfg.Server.TLSAutoSelfSign,
		HTTPRedirect: scheme == "https" && config.ServerHTTPRedirectEnabled(&cfg.Server),
	})

	// when MCP is enabled and auth_header_value is empty, auto-generate a random key and write back to config
	if err := config.EnsureMCPAuth(cp, cfg); err != nil {
		fmt.Printf("Failed to configure MCP authentication: %v\n", err)
		return
	}
	if cfg.MCP.Enabled {
		config.PrintMCPConfigJSON(cfg.MCP)
	}

	// initialize log
	log := logger.New(cfg.Log.Level, cfg.Log.Output, logger.DiagnosticOptions{
		Dir:           cfg.Log.DiagnosticDir,
		Disabled:      cfg.Log.DiagnosticDisabled,
		RetentionDays: cfg.Log.DiagnosticRetentionDays,
	})
	defer func() { _ = log.Sync() }()

	if err := configureProcessIsolation(cfg); err != nil {
		log.Fatal("process isolationinitialization failed", "error", err)
	}

	probeCtx, probeCancel := context.WithTimeout(context.Background(), 15*time.Second)
	backend, probeErr := processguard.Check(probeCtx)
	probeCancel()
	if probeErr != nil {
		log.Fatal("process isolationstartcheckfailed", "error", probeErr)
	}
	log.Info("task process isolation ready", zap.String("backend", backend))

	// create cancellable root context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// listen for system signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// create application
	application, err := app.New(cfg, log, cp)
	if err != nil {
		log.Fatal("application initialization failed", "error", err)
	}

	if isDemoSubcommand || *demoFlag {
		baseURL := fmt.Sprintf("%s://%s:%d", scheme, cfg.Server.Host, port)
		if cfg.Server.Host == "" || cfg.Server.Host == "0.0.0.0" {
			baseURL = fmt.Sprintf("%s://localhost:%d", scheme, port)
		}
		demoDetails, err := demo.SeedDemoProject(application.DB(), application.Auth().JWTSecret(), baseURL)
		if err != nil {
			log.Warn("failed to fully seed demo project", zap.Error(err))
		} else {
			fmt.Println("\n" + strings.Repeat("=", 75))
			fmt.Println("              KESTREL AUTOMATED PENTEST — DEMO MODE READY")
			fmt.Println(strings.Repeat("=", 75))
			fmt.Printf("  Target Target:     %s (OWASP Juice Shop)\n", demoDetails.TargetURL)
			fmt.Printf("  Direct Web UI:     %s\n", demoDetails.DirectLoginURL)
			fmt.Printf("  Admin Credentials: %s / %s\n", demoDetails.AdminUsername, demoDetails.AdminPassword)
			fmt.Printf("  Report Permalink:  %s\n", demoDetails.ShareURL)
			fmt.Printf("  Live Status Badge: %s\n", demoDetails.BadgeURL)
			fmt.Println(strings.Repeat("=", 75))
			fmt.Printf("  Server listening on %s. Explore the seeded findings in the UI!\n", baseURL)
			fmt.Println(strings.Repeat("=", 75) + "\n")
		}
	}

	// listen for signals in background
	go func() {
		sig := <-sigCh
		log.Info("received system signal, starting graceful shutdown: " + sig.String())
		cancel()
	}()

	// start server (passing context for graceful shutdown support)
	if err := application.RunWithContext(ctx); err != nil {
		// shutdown caused by context cancellation is not treated as an error
		if ctx.Err() != nil {
			log.Info("server gracefully shut down")
		} else {
			log.Fatal("server startup failed", "error", err)
		}
	}
	application.Shutdown()
}

func runResetAdminPassword(cfg *config.Config) error {
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		dbPath = "data/conversations.db"
	}
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("database does not exist: %s; start the service once to initialize it first", dbPath)
		}
		return err
	}

	fmt.Println("Reset built-in admin password")
	fmt.Println()

	password, err := readHiddenPassword("New admin password: ")
	if err != nil {
		return err
	}
	password = strings.TrimSpace(password)
	if len(password) < 8 {
		return fmt.Errorf("new password must be at least 8 characters")
	}
	confirm, err := readHiddenPassword("Confirm new password: ")
	if err != nil {
		return err
	}
	if password != strings.TrimSpace(confirm) {
		return fmt.Errorf("passwords do not match")
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}

	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	admin, err := db.GetRBACUserByUsername("admin")
	if err != nil {
		return fmt.Errorf("built-in admin account was not found; start the service once to initialize it first: %w", err)
	}
	if !admin.IsBuiltin {
		return fmt.Errorf("admin account is not built in; refusing to reset it")
	}
	if err := db.UpdateRBACAdminPassword(hash); err != nil {
		return err
	}
	if err := db.SetRevocationCutoff(time.Now().UTC()); err != nil {
		return fmt.Errorf("failed to revoke existing login sessions: %w", err)
	}

	fmt.Println()
	fmt.Println("Admin password has been reset.")
	fmt.Println("All existing login sessions have been revoked.")
	return nil
}

func runAuditCheckpoint(cfg *config.Config, outFile string) error {
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		dbPath = "data/conversations.db"
	}
	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	cp, err := db.CreateAuditCheckpoint()
	if err != nil {
		return fmt.Errorf("audit checkpoint creation failed: %w", err)
	}

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode checkpoint: %w", err)
	}

	if outFile != "" {
		if err := os.WriteFile(outFile, data, 0644); err != nil {
			return fmt.Errorf("failed to write audit checkpoint to %s: %w", outFile, err)
		}
		fmt.Fprintf(os.Stdout, "SUCCESS: Created audit checkpoint for head %s (%d records) -> %s\n", cp.HeadHash, cp.ChainedCount, outFile)
	} else {
		fmt.Println(string(data))
	}
	return nil
}

func runAuditVerify(cfg *config.Config, anchorFile string) error {
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		dbPath = "data/conversations.db"
	}
	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if anchorFile != "" {
		content, err := os.ReadFile(anchorFile)
		if err != nil {
			return fmt.Errorf("failed to read external checkpoint file %s: %w", anchorFile, err)
		}
		var cp database.AuditCheckpoint
		if err := json.Unmarshal(content, &cp); err != nil {
			return fmt.Errorf("failed to parse checkpoint file %s: %w", anchorFile, err)
		}
		valid, err := db.VerifyAuditCheckpoint(&cp)
		if err != nil || !valid {
			return fmt.Errorf("audit verification against external anchor failed: %w", err)
		}
		fmt.Fprintf(os.Stdout, "SUCCESS: Audit log chain verified against external anchor %s (checkpoint head=%s, verified=%d records)\n", anchorFile, cp.HeadHash, cp.ChainedCount)
		return nil
	}

	valid, err := db.VerifyAuditLogChain()
	if err != nil || !valid {
		return fmt.Errorf("audit chain integrity verification failed: %w", err)
	}
	fmt.Println("SUCCESS: Audit log chain verified (internal consistency and pinned head valid)")
	return nil
}

func readHiddenPassword(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return string(password), nil
}

func configureProcessIsolation(cfg *config.Config) error {
	isolation := cfg.Security.ProcessIsolation
	return processguard.Configure(processguard.Options{Mode: isolation.Mode, CgroupRoot: isolation.CgroupRoot, MaxProcesses: isolation.MaxProcesses, MemoryMaxBytes: isolation.MemoryMaxBytes, CPUQuotaMicros: isolation.CPUQuotaMicros})
}

type qualityGateFailedError struct {
	threshold string
}

func (e *qualityGateFailedError) Error() string {
	return fmt.Sprintf("quality gate failed: detected vulnerabilities with severity >= %s", e.threshold)
}

type retestExitError struct {
	exitCode int
}

func (e *retestExitError) Error() string {
	return fmt.Sprintf("retest completed with exit code %d", e.exitCode)
}

func runExportSarif(cfg *config.Config, projectID, outFile, failOn string, seedFixture bool) error {
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		dbPath = "data/conversations.db"
	}
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if seedFixture {
		sampleVuln := &database.Vulnerability{
			ID:             "kestrel-fixture-1",
			Title:          "Sample Security Finding (CI Verification Fixture)",
			Description:    "Automated SARIF pipeline verification fixture generated during CI scan.",
			Severity:       "low",
			Status:         "confirmed",
			Type:           "CWE-200",
			Target:         "internal/app/app.go:1",
			Evidence:       "Automated CI validation test evidence",
			Recommendation: "Verify CI export pipeline integrity",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		_, _ = db.CreateVulnerability(sampleVuln)
	}

	filter := database.VulnerabilityListFilter{
		ProjectID: strings.TrimSpace(projectID),
	}
	access := database.RBACListAccess{Scope: database.RBACScopeAll}

	total, err := db.CountVulnerabilitiesForAccess(filter, access)
	if err != nil {
		return fmt.Errorf("failed to count vulnerabilities: %w", err)
	}

	vulns := make([]*database.Vulnerability, 0)
	if total > 0 {
		vulns, err = db.ListVulnerabilitiesForAccess(total, 0, filter, access)
		if err != nil {
			return fmt.Errorf("failed to list vulnerabilities: %w", err)
		}
	}

	sarifData, err := report.ToJSON(vulns, "", true)
	if err != nil {
		return fmt.Errorf("failed to generate SARIF: %w", err)
	}

	if outFile != "" {
		if err := os.WriteFile(outFile, sarifData, 0644); err != nil {
			return fmt.Errorf("failed to write SARIF output to %s: %w", outFile, err)
		}
		fmt.Fprintf(os.Stderr, "Exported %d vulnerabilities to SARIF: %s\n", len(vulns), outFile)
	} else {
		fmt.Println(string(sarifData))
	}

	if failOn != "" {
		thresholdMet := false
		for _, v := range vulns {
			if strings.HasPrefix(v.ID, "kestrel-fixture-") {
				continue // do not trip CI quality gate on seeded test validation fixture
			}
			matched, err := meetsSeverityThreshold(v.Severity, failOn)
			if err != nil {
				return err
			}
			if matched {
				thresholdMet = true
				break
			}
		}
		if thresholdMet {
			return &qualityGateFailedError{threshold: failOn}
		}
	}

	return nil
}

func meetsSeverityThreshold(severity, threshold string) (bool, error) {
	weights := map[string]int{
		"critical": 5,
		"high":     4,
		"medium":   3,
		"low":      2,
		"info":     1,
	}
	tKey := strings.ToLower(strings.TrimSpace(threshold))
	t, ok := weights[tKey]
	if !ok {
		return false, fmt.Errorf("invalid -sarif-fail-on threshold %q (must be one of: critical, high, medium, low, info)", threshold)
	}
	sKey := strings.ToLower(strings.TrimSpace(severity))
	v := weights[sKey]
	return v >= t, nil
}

func runRetestVuln(cfg *config.Config, vulnID, targetOverride string) error {
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		dbPath = "data/conversations.db"
	}
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("database does not exist: %s", dbPath)
	}

	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	vuln, err := db.GetVulnerability(vulnID)
	if err != nil {
		return fmt.Errorf("failed to retrieve vulnerability %s: %w", vulnID, err)
	}
	if vuln == nil {
		return fmt.Errorf("vulnerability %s not found", vulnID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	result, err := report.RunVulnerabilityRetest(ctx, vuln, targetOverride)
	if err != nil {
		return fmt.Errorf("retest failed to execute: %w", err)
	}

	if result.IsResolved {
		vuln.Status = "fixed"
		vuln.RetestNotes = fmt.Sprintf("Automated CI retest verified finding as RESOLVED on %s. %s", result.Timestamp, result.Message)
		_ = db.UpdateVulnerability(vuln.ID, vuln)
		fmt.Fprintf(os.Stdout, "SUCCESS: Vulnerability %s verified as RESOLVED (status=fixed, exit_code=%d)\n", vuln.ID, result.ExitCode)
		if result.Output != "" {
			fmt.Println(result.Output)
		}
		return nil
	} else if result.ExitCode == 0 {
		vuln.Status = "confirmed"
		vuln.RetestNotes = fmt.Sprintf("Automated CI retest confirmed vulnerability is STILL ACTIVE on %s.", result.Timestamp)
		_ = db.UpdateVulnerability(vuln.ID, vuln)
		fmt.Fprintf(os.Stderr, "FAILURE: Vulnerability %s is STILL ACTIVE (status=confirmed, exit_code=%d)\n", vuln.ID, result.ExitCode)
		if result.Output != "" {
			fmt.Fprintln(os.Stderr, result.Output)
		}
		return &retestExitError{exitCode: 1}
	} else {
		fmt.Fprintf(os.Stderr, "ERROR: Vulnerability %s retest inconclusive (exit_code=%d): %s\n", vuln.ID, result.ExitCode, result.Message)
		if result.Output != "" {
			fmt.Fprintln(os.Stderr, result.Output)
		}
		return &retestExitError{exitCode: 2}
	}
}

func runReportShare(cfg *config.Config, projectID string, ttl time.Duration, passphrase, baseURL string) error {
	dbPath := strings.TrimSpace(cfg.Database.Path)
	if dbPath == "" {
		dbPath = "data/conversations.db"
	}
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("database does not exist: %s", dbPath)
	}

	db, err := database.NewDB(dbPath, zap.NewNop())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	proj, err := db.GetProject(projectID)
	if err != nil {
		return fmt.Errorf("failed to retrieve project: %w", err)
	}
	if proj == nil {
		return fmt.Errorf("project %q not found", projectID)
	}

	jwtSecret := []byte(cfg.Auth.JWTSecret)
	if len(jwtSecret) == 0 {
		jwtSecret = []byte("kestrel-share-signing-key-default")
	}

	expiresAt := time.Now().Add(ttl)
	token, tokenHash, err := report.CreateShareToken(projectID, expiresAt, jwtSecret)
	if err != nil {
		return fmt.Errorf("failed to generate share token: %w", err)
	}

	var passphraseHash string
	if strings.TrimSpace(passphrase) != "" {
		hsh, err := security.HashPassword(strings.TrimSpace(passphrase))
		if err != nil {
			return fmt.Errorf("failed to hash passphrase: %w", err)
		}
		passphraseHash = hsh
	}

	share := &database.ReportShare{
		ProjectID:      projectID,
		TokenHash:      tokenHash,
		PassphraseHash: passphraseHash,
		ExpiresAt:      expiresAt,
		CreatedAt:      time.Now(),
	}
	if err := db.CreateReportShare(share); err != nil {
		return fmt.Errorf("failed to store share link: %w", err)
	}

	if baseURL == "" {
		scheme := "http"
		if config.MainWebUIUsesHTTPS(&cfg.Server) {
			scheme = "https"
		}
		host := cfg.Server.Host
		if host == "" || host == "0.0.0.0" {
			host = "localhost"
		}
		port := cfg.Server.Port
		if port <= 0 {
			port = 8080
		}
		baseURL = fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}
	baseURL = strings.TrimRight(baseURL, "/")

	shareURL := fmt.Sprintf("%s/r/%s", baseURL, token)
	badgeURL := fmt.Sprintf("%s/badge/%s?token=%s", baseURL, projectID, token)

	fmt.Printf("\n=== Kestrel Report Share Permalink ===\n")
	fmt.Printf("Project:      %s (%s)\n", proj.Name, proj.ID)
	fmt.Printf("Share Link:   %s\n", shareURL)
	fmt.Printf("Status Badge: %s\n", badgeURL)
	if passphrase != "" {
		fmt.Printf("Passphrase:   %s\n", passphrase)
	}
	fmt.Printf("Expires At:   %s\n", expiresAt.Format(time.RFC3339))
	fmt.Printf("\nMarkdown Badge:\n[![Kestrel Security Status](%s)](%s)\n\n", badgeURL, shareURL)
	return nil
}
