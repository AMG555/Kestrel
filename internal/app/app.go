package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"kestrel/internal/agent"
	"kestrel/internal/audit"
	"kestrel/internal/auth"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/handler"
	"kestrel/internal/knowledge"
	"kestrel/internal/llm"
	"kestrel/internal/mcp"
	"kestrel/internal/middleware"
	"kestrel/internal/report"
	"kestrel/internal/robot"
	"kestrel/internal/storage"
	"kestrel/internal/termout"
	"kestrel/internal/toolrecipe"
	"kestrel/internal/vision"
	"kestrel/internal/workflow"
)

// App is the root application container.
type App struct {
	cfg      *config.Config
	db       *database.DB
	auth     *auth.Service
	audit    *audit.Service
	registry *mcp.Registry
	runner   *agent.Runner
	kb       *knowledge.Service
	workflow *workflow.Engine
	recipes  *toolrecipe.Registry
	robot    *robot.Service
	llm      *llm.Manager
	router   *gin.Engine
	logger   *zap.Logger
}

// New builds an App from the given config and logger.
func New(cfg *config.Config, logger *zap.Logger) (*App, error) {
	// Open database.
	db, err := database.New(cfg.Database.Path, logger)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Build auth and audit services.
	authSvc := auth.New(db, cfg.Auth.JWTSecret, cfg.Auth.SessionDurationHours)
	auditSvc := audit.NewService(db, logger)

	// Seed the admin user if the database is empty.
	if err := seedAdmin(db, authSvc, logger); err != nil {
		return nil, fmt.Errorf("seeding admin: %w", err)
	}

	// Seed default system roles.
	if err := seedSystemRoles(db, logger); err != nil {
		return nil, fmt.Errorf("seeding system roles: %w", err)
	}

	// Compile the toolguard policy from config (defaults to government-domain rule).
	tgCfg := cfg.EffectiveToolGuard()
	registry := mcp.NewRegistryWithGuard(&cfg.MCP, &tgCfg, logger)

	// Build knowledge service.
	kb := knowledge.New(&cfg.Knowledge, db, logger)

	// Build LLM manager (multi-provider: OpenAI / Anthropic / Ollama / stub).
	llmMgr := llm.NewManager(&cfg.AI, logger)

	// Build agent runner.
	runner := agent.NewRunner(&cfg.Agent, db, registry, llmMgr, logger)

	// Cancel any orphaned running tool executions from a previous run.
	if _, err := db.CancelOrphanedRunningToolExecutions(time.Now(), "server_restart"); err != nil {
		logger.Warn("could not cancel orphaned tool executions", zap.Error(err))
	}

	// Build workflow engine.
	wfEngine := workflow.NewEngine(db, runner, registry, logger)

	// Build router.
	if cfg.Log.Level != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.CORS(cfg.Server.CORSOrigins))

	// Register vision inspection tool.
	vision.RegisterVisionTool(registry)

	// Load curated tool recipes catalog.
	recipes, err := toolrecipe.NewRegistry("./tools")
	if err != nil {
		logger.Warn("could not load tool recipes", zap.Error(err))
	}

	// Initialize robot notification service.
	robotSvc := robot.NewService(robot.Config{
		Enabled:              true,
		NotifyOnCriticalVuln: true,
		NotifyOnHITL:         true,
		NotifyOnTaskDone:     true,
	}, logger)

	a := &App{
		cfg:      cfg,
		db:       db,
		auth:     authSvc,
		audit:    auditSvc,
		registry: registry,
		runner:   runner,
		kb:       kb,
		workflow: wfEngine,
		recipes:  recipes,
		robot:    robotSvc,
		llm:      llmMgr,
		router:   router,
		logger:   logger,
	}
	a.registerRoutes()
	return a, nil
}

// Serve starts the HTTP or HTTPS server and blocks until ctx is cancelled.
func (a *App) Serve(ctx context.Context) error {
	srv := &http.Server{
		Addr:         a.cfg.Server.Address(),
		Handler:      a.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if a.cfg.Server.TLSActive() {
		tlsCfg, err := a.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("building TLS config: %w", err)
		}
		srv.TLSConfig = tlsCfg
	}

	// Start storage retention cleaner in background.
	storage.StartRetentionWorker(ctx, "./data/spill", 30*time.Minute, 24*time.Hour, a.logger)

	// Start audit retention worker in background (90-day retention, purge daily).
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		// Initial purge on startup
		_, _ = a.audit.PurgeExpired(90)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = a.audit.PurgeExpired(90)
			}
		}
	}()

	// Shutdown on context cancellation.
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		_ = a.db.Close()
	}()

	scheme := "http"
	if a.cfg.Server.TLSActive() {
		scheme = "https"
	}
	termout.PrintStartupWebUI(termout.StartupWebUIOptions{
		Scheme:     scheme,
		Host:       a.cfg.Server.Host,
		Port:       a.cfg.Server.Port,
		SelfSigned: a.cfg.Server.TLSAutoSelfSign,
	})
	a.logger.Info("Kestrel listening",
		zap.String("address", fmt.Sprintf("%s://%s", scheme, srv.Addr)),
		zap.String("status", "under development"),
	)

	if a.cfg.Server.TLSActive() {
		return srv.ListenAndServeTLS("", "")
	}
	return srv.ListenAndServe()
}

// registerRoutes wires all API routes onto the Gin engine.
func (a *App) registerRoutes() {
	// Rate limiter for login endpoint.
	loginRL := middleware.NewRateLimiter(
		a.cfg.RateLimit.LoginMaxAttempts,
		a.cfg.RateLimit.LoginWindowSecs,
	)

	// Handlers.
	authH    := handler.NewAuthHandler(a.auth, a.db)
	userH    := handler.NewUserHandler(a.db)
	roleH    := handler.NewRoleHandler(a.db)
	assetH   := handler.NewAssetHandler(a.db)
	vulnH    := handler.NewVulnHandler(a.db)
	auditH   := handler.NewAuditHandler(a.db)
	agentH   := handler.NewAgentHandler(a.runner, a.db, a.registry, a.logger)
	kbH      := handler.NewKnowledgeHandler(a.kb)
	projH    := handler.NewProjectHandler(a.db)
	batchH   := handler.NewBatchHandler(a.db, a.runner, a.registry, a.logger)
	hitlH    := handler.NewHITLHandler(a.db)
	convH    := handler.NewConversationHandler(a.db)
	mcpSrvH  := handler.NewMCPServerHandler(a.db)
	workflowH  := handler.NewWorkflowHandler(a.db, a.workflow, a.logger)
	reportH    := handler.NewReportHandler(report.NewGenerator(a.db))
	sessionH   := handler.NewSessionHandler(a.db)
	toolguardH := handler.NewToolGuardHandler(a.registry, a.db)
	skillsH    := handler.NewSkillsHandler("./skills")
	toolCatalogH := handler.NewToolCatalogHandler(a.recipes)
	robotH     := handler.NewRobotHandler(a.robot)
	terminalH  := handler.NewTerminalHandler(a.db, a.registry, a.logger)
	monitorH   := handler.NewMonitorHandler(a.db, a.registry, a.cfg.Database.Path)
	openapiH   := handler.NewOpenAPIHandler()
	c2H        := handler.NewC2Handler(a.db)
	osintH     := handler.NewOSINTHandler(a.db, a.llm, a.logger)
	webshellH  := handler.NewWebShellHandler(a.db, a.logger)

	// Disclaimer / consent check on all routes.
	a.router.Use(middleware.AuditContext())

	// Serve the SPA.
	a.router.Static("/web", "./web/dist")
	a.router.NoRoute(func(c *gin.Context) {
		c.File("./web/dist/index.html")
	})

	api := a.router.Group("/api")

	// Public endpoints.
	api.GET("/system/info", agentH.SystemInfo)
	api.POST("/auth/login", middleware.LoginRateLimit(loginRL), authH.Login)

	// Authenticated endpoints.
	authed := api.Group("/")
	authed.Use(middleware.Auth(a.auth))
	authed.Use(middleware.RequirePasswordChange())

	authed.POST("/auth/logout", authH.Logout)
	authed.POST("/auth/change-password", authH.ChangePassword)
	authed.GET("/auth/me", authH.Me)

	// Dashboard.
	authed.GET("/dashboard/stats", agentH.DashboardStats)

	// Users (admin-only in practice; simplified here to permission check).
	authed.GET("/users", userH.ListUsers)
	authed.POST("/users", userH.CreateUser)
	authed.GET("/users/:id", userH.GetUser)
	authed.PATCH("/users/:id", userH.UpdateUser)
	authed.DELETE("/users/:id", userH.DeleteUser)
	authed.POST("/users/:id/roles", userH.AssignRole)
	authed.DELETE("/users/:id/roles/:role_id", userH.RevokeRole)

	// Roles.
	authed.GET("/roles", roleH.ListRoles)
	authed.POST("/roles", roleH.CreateRole)
	authed.GET("/roles/:id", roleH.GetRole)
	authed.PATCH("/roles/:id", roleH.UpdateRole)
	authed.DELETE("/roles/:id", roleH.DeleteRole)

	// Assets.
	authed.GET("/assets", assetH.ListAssets)
	authed.GET("/assets/export.csv", assetH.ExportAssetsCSV)
	authed.POST("/assets", assetH.CreateAsset)
	authed.GET("/assets/:id", assetH.GetAsset)
	authed.DELETE("/assets/:id", assetH.DeleteAsset)

	// Vulnerabilities.
	authed.GET("/vulnerabilities", vulnH.ListVulnerabilities)
	authed.GET("/vulnerabilities/export.csv", vulnH.ExportVulnsCSV)
	authed.POST("/vulnerabilities", vulnH.CreateVulnerability)
	authed.GET("/vulnerabilities/:id", vulnH.GetVulnerability)
	authed.PATCH("/vulnerabilities/:id", vulnH.UpdateVulnerability)
	authed.DELETE("/vulnerabilities/:id", vulnH.DeleteVulnerability)

	// Audit logs.
	authed.GET("/audit", auditH.ListAuditLogs)
	authed.GET("/audit/summary", auditH.AuditSummary)
	authed.GET("/audit/export.csv", auditH.ExportAuditCSV)

	// Tools.
	authed.GET("/tools", agentH.ListTools)
	authed.POST("/tools/:name/execute", agentH.ExecuteTool)

	// Agent.
	authed.POST("/agent/run", agentH.Run)
	authed.GET("/agent/stream", agentH.RunStream)

	// Knowledge base.
	authed.GET("/knowledge/documents", kbH.ListDocuments)
	authed.POST("/knowledge/ingest", kbH.IngestText)
	authed.POST("/knowledge/query", kbH.Query)
	authed.DELETE("/knowledge/documents/:id", kbH.DeleteDocument)

	// Projects.
	authed.GET("/projects", projH.ListProjects)
	authed.POST("/projects", projH.CreateProject)
	authed.GET("/projects/:id", projH.GetProject)
	authed.PATCH("/projects/:id", projH.UpdateProject)
	authed.DELETE("/projects/:id", projH.DeleteProject)
	authed.GET("/projects/:id/facts", projH.ListProjectFacts)
	authed.POST("/projects/:id/facts", projH.UpsertProjectFact)
	authed.DELETE("/projects/:id/facts/:fact_id", projH.DeleteProjectFact)
	authed.GET("/projects/:id/attack-chain", projH.GetAttackChain)
	authed.POST("/projects/:id/attack-chain/nodes", projH.AddChainNode)
	authed.POST("/projects/:id/attack-chain/edges", projH.AddChainEdge)
	authed.GET("/projects/:id/report", reportH.GenerateReport)

	// Batch task queues.
	authed.GET("/batch/queues", batchH.ListQueues)
	authed.POST("/batch/queues", batchH.CreateQueue)
	authed.GET("/batch/queues/:id", batchH.GetQueue)
	authed.POST("/batch/queues/:id/run", batchH.RunQueue)
	authed.POST("/batch/queues/:id/cancel", batchH.CancelQueue)
	authed.DELETE("/batch/queues/:id", batchH.DeleteQueue)

	// HITL approvals.
	authed.GET("/hitl/pending", hitlH.ListPending)
	authed.POST("/hitl/:id/decide", hitlH.Decide)

	// Conversations.
	authed.GET("/conversations", convH.ListConversations)
	authed.POST("/conversations", convH.CreateConversation)
	authed.PATCH("/conversations/:id", convH.UpdateConversation)
	authed.DELETE("/conversations/:id", convH.DeleteConversation)
	authed.GET("/conversations/:id/messages", convH.GetConversationMessages)

	// MCP server management.
	authed.GET("/mcp/servers", mcpSrvH.ListMCPServers)
	authed.POST("/mcp/servers", mcpSrvH.UpsertMCPServer)
	authed.DELETE("/mcp/servers/:id", mcpSrvH.DeleteMCPServer)
	authed.POST("/mcp/servers/:id/reset-circuit", mcpSrvH.ResetCircuit)

	// Agent sessions.
	authed.GET("/sessions", sessionH.ListSessions)
	authed.GET("/sessions/:id", sessionH.GetSession)
	authed.PATCH("/sessions/:id", sessionH.UpdateSession)
	authed.DELETE("/sessions/:id", sessionH.DeleteSession)

	// Tool executions.
	authed.GET("/tool-executions", sessionH.ListToolExecutions)

	// ToolGuard config.
	authed.GET("/tool-guard/config", toolguardH.GetConfig)
	authed.PUT("/tool-guard/config", toolguardH.UpdateConfig)

	// Token usage stats.
	authed.GET("/stats/token-usage", func(c *gin.Context) {
		summary, err := a.db.TokenUsageSummary()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve token usage"})
			return
		}
		c.JSON(http.StatusOK, summary)
	})

	// Workflows.
	authed.GET("/workflows", workflowH.ListWorkflows)
	authed.POST("/workflows", workflowH.CreateWorkflow)
	authed.GET("/workflows/:id", workflowH.GetWorkflow)
	authed.PATCH("/workflows/:id", workflowH.UpdateWorkflow)
	authed.DELETE("/workflows/:id", workflowH.DeleteWorkflow)
	authed.POST("/workflows/:id/run", workflowH.RunWorkflow)
	authed.GET("/workflows/runs/:run_id", workflowH.GetRun)

	// Skills.
	authed.GET("/skills", skillsH.ListSkills)
	authed.GET("/skills/:id", skillsH.GetSkill)

	// Tool Recipes Catalog.
	authed.GET("/tools/catalog", toolCatalogH.ListCatalog)
	authed.GET("/tools/catalog/:name", toolCatalogH.GetCatalogItem)

	// Robots & Webhooks.
	authed.GET("/robots/config", robotH.GetConfig)
	authed.PUT("/robots/config", robotH.UpdateConfig)
	authed.POST("/robots/test", robotH.TestNotification)

	// Attack Chain Promotion.
	authed.POST("/projects/:id/attack-chain/promote", projH.PromoteAttackChain)

	// OpenAPI Spec.
	api.GET("/openapi/spec", openapiH.GetSpec)

	// Interactive Operator Terminal.
	authed.POST("/terminal/exec", terminalH.ExecCommand)

	// Runtime Monitor & Telemetry.
	authed.GET("/monitor/status", monitorH.GetStatus)

	// C2 Emulation.
	authed.GET("/c2/listeners", c2H.ListListeners)
	authed.POST("/c2/listeners", c2H.CreateListener)
	authed.GET("/c2/beacons", c2H.ListBeacons)
	authed.POST("/c2/beacons/:id/tasks", c2H.QueueTask)

	// OSINT & Cyberspace Reconnaissance.
	authed.POST("/osint/parse", osintH.ParseNLQuery)
	authed.POST("/osint/search", osintH.Search)
	authed.POST("/osint/import-assets", osintH.ImportAssets)

	// WebShell & Target Probes.
	authed.GET("/webshell/connections", webshellH.ListConnections)
	authed.POST("/webshell/connections", webshellH.CreateConnection)
	authed.DELETE("/webshell/connections/:id", webshellH.DeleteConnection)
	authed.POST("/webshell/:id/probe", webshellH.ProbeConnection)
	authed.POST("/webshell/:id/exec", webshellH.ExecCommand)
}

// buildTLSConfig returns a *tls.Config, generating a self-signed cert if needed.
func (a *App) buildTLSConfig() (*tls.Config, error) {
	if a.cfg.Server.TLSCertPath != "" && a.cfg.Server.TLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(a.cfg.Server.TLSCertPath, a.cfg.Server.TLSKeyPath)
		if err != nil {
			return nil, err
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	}

	if a.cfg.Server.TLSAutoSelfSign {
		cert, err := generateSelfSignedCert(a.cfg.Server.Host)
		if err != nil {
			return nil, err
		}
		a.logger.Warn("Using self-signed certificate — not suitable for production")
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	}

	return nil, fmt.Errorf("TLS is enabled but no certificate is configured")
}

// generateSelfSignedCert produces an in-memory self-signed TLS certificate.
func generateSelfSignedCert(host string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "kestrel-local"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost", host},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(key)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// seedAdmin creates the initial admin user if no users exist.
func seedAdmin(db *database.DB, authSvc *auth.Service, logger *zap.Logger) error {
	exists, err := db.AdminExists()
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	// Generate a random initial password.
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	password := fmt.Sprintf("%x", buf)

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	user, err := db.CreateUser("admin", hash, "", "Administrator")
	if err != nil {
		return err
	}

	// Ensure admin does not have to change password on first scripted use,
	// but does require it in interactive mode.
	_ = user

	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	logger.Info("  ADMIN SETUP REQUIRED")
	logger.Info(fmt.Sprintf("  Username: admin"))
	logger.Info(fmt.Sprintf("  Password: %s", password))
	logger.Info("  Change this password immediately after first login.")
	logger.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// seedSystemRoles creates the built-in system roles if they don't exist.
func seedSystemRoles(db *database.DB, logger *zap.Logger) error {
	type systemRole struct {
		name         string
		description  string
		permissions  []string
		allowedTools []string
		hitlMode     string
	}

	roles := []systemRole{
		{
			name:        "admin",
			description: "Platform administrator — full access",
			permissions: []string{"*"},
			allowedTools: []string{"*"},
			hitlMode:    "auto",
		},
		{
			name:        "operator",
			description: "Security operator — can run recon tools with auto-approval",
			permissions: []string{"read:assets", "write:assets", "read:vulns", "write:vulns", "run:agent", "use:tools"},
			allowedTools: []string{"subdomain_enum", "http_probe", "dns_lookup"},
			hitlMode:    "auto",
		},
		{
			name:        "analyst",
			description: "Security analyst — read-only access plus tool execution requiring approval",
			permissions: []string{"read:assets", "read:vulns", "run:agent", "use:tools"},
			allowedTools: []string{"subdomain_enum", "http_probe", "dns_lookup"},
			hitlMode:    "require_approval",
		},
		{
			name:        "viewer",
			description: "Read-only viewer — cannot execute tools or agents",
			permissions: []string{"read:assets", "read:vulns"},
			allowedTools: []string{},
			hitlMode:    "require_approval",
		},
	}

	for _, sr := range roles {
		existing, err := db.GetRoleByName(sr.name)
		if err != nil {
			return err
		}
		if existing != nil {
			continue
		}
		if _, err := db.CreateRole(sr.name, sr.description, true, sr.permissions, sr.allowedTools, sr.hitlMode); err != nil {
			return fmt.Errorf("creating system role %q: %w", sr.name, err)
		}
		logger.Info("created system role", zap.String("name", sr.name))
	}

	// Assign admin role to the admin user.
	adminUser, err := db.GetUserByUsername("admin")
	if err != nil || adminUser == nil {
		return nil
	}
	adminRole, err := db.GetRoleByName("admin")
	if err != nil || adminRole == nil {
		return nil
	}
	_ = db.AssignRole(adminUser.ID, adminRole.ID, "system")
	return nil
}
