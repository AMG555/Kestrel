package app

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"kestrel/internal/agent"
	"kestrel/internal/audit"
	"kestrel/internal/authctx"
	"kestrel/internal/c2"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/einoobserve"
	"kestrel/internal/handler"
	"kestrel/internal/hitl"
	"kestrel/internal/knowledge"
	"kestrel/internal/logger"
	"kestrel/internal/mcp"
	"kestrel/internal/monitor"
	"kestrel/internal/multiagent"
	"kestrel/internal/robot"
	"kestrel/internal/security"
	"kestrel/internal/skillpackage"
	"kestrel/internal/storage"
	"kestrel/internal/toolguard"
)

// App is the main application struct
type App struct {
	config             *config.Config
	logger             *logger.Logger
	router             *gin.Engine
	mcpServer          *mcp.Server
	externalMCPMgr     *mcp.ExternalMCPManager
	agent              *agent.Agent
	executor           *security.Executor
	db                 *database.DB
	knowledgeDB        *database.DB // knowledge base database connection (if using a separate database)
	auth               *security.AuthManager
	knowledgeManager   *knowledge.Manager        // knowledge base manager (for dynamic initialization)
	knowledgeRetriever *knowledge.Retriever      // knowledge base retriever (for dynamic initialization)
	knowledgeIndexer   *knowledge.Indexer        // knowledge base indexer (for dynamic initialization)
	knowledgeHandler   *handler.KnowledgeHandler // knowledge base handler (for dynamic initialization)
	agentHandler       *handler.AgentHandler     // agent handler (for updating knowledge base manager)
	robotHandler       *handler.RobotHandler     // bot handler (DingTalk/Feishu/WeCom etc.)
	robotMu            sync.Mutex                // protects the long-lived bot connection cancel funcs
	dingCancel         context.CancelFunc        // DingTalk stream cancel function, used to restart on config change
	larkCancel         context.CancelFunc        // Feishu long-connection cancel function, used to restart on config change
	wechatCancel       context.CancelFunc        // WeChat iLink long-poll cancel function
	telegramCancel     context.CancelFunc        // Telegram long-poll cancel function
	slackCancel        context.CancelFunc        // Slack Socket Mode cancel function
	discordCancel      context.CancelFunc        // Discord Gateway cancel function
	qqCancel           context.CancelFunc        // QQ WebSocket cancel function
	alertCancel        context.CancelFunc        // vulnerability alert persistent delivery worker
	c2Manager          *c2.Manager               // C2 manager (nil when C2 is not enabled)
	c2Watchdog         *c2.SessionWatchdog       // C2 session watchdog
	c2WatchdogCancel   context.CancelFunc        // watchdog cancel function
	c2Handler          *handler.C2Handler        // C2 REST (lifecycle synced with Manager)
	storageHandler     *handler.StorageHandler   // runtime storage usage stats and garbage cleanup
	auditSvc           *audit.Service
}

// New creates a new application instance
func New(cfg *config.Config, log *logger.Logger, configPath string) (*App, error) {
	toolGuard, err := toolguard.NewManager(cfg.EffectiveToolGuard())
	if err != nil {
		return nil, fmt.Errorf("initializing call interception rules: %w", err)
	}
	if err := multiagent.InitADK(); err != nil {
		return nil, fmt.Errorf("initializing Eino ADK: %w", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// Restrict trusted proxies to prevent client IP spoofing via X-Forwarded-For
	trustedProxies := cfg.Server.TrustedProxies
	if len(trustedProxies) == 0 {
		trustedProxies = []string{"127.0.0.1", "::1"}
	}
	if err := router.SetTrustedProxies(trustedProxies); err != nil {
		log.Warn("failed to configure trusted proxies", zap.Error(err))
	}

	// CORS middleware
	router.Use(corsMiddleware(cfg.Server.CORSAllowedOrigins))
	// Security headers middleware
	router.Use(securityHeadersMiddleware(config.MainWebUIUsesHTTPS(&cfg.Server)))

	// initialize database
	dbPath := cfg.Database.Path
	if dbPath == "" {
		dbPath = "data/conversations.db"
	}

	// ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create database directory failed: %w", err)
	}

	db, err := database.NewDB(dbPath, log.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	db.StartJanitor(context.Background(), 30*time.Minute)

	// auth manager (RBAC mounted after database initialization)
	jwtSecret := strings.TrimSpace(cfg.Auth.JWTSecret)
	if jwtSecret == "" {
		jwtSecret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	authManager := security.NewAuthManager(cfg.Auth.SessionDurationHours, jwtSecret)
	if generatedPassword, err := authManager.AttachRBACStore(db); err != nil {
		return nil, fmt.Errorf("initializing RBAC failed: %w", err)
	} else if generatedPassword != "" {
		config.PrintBootstrapAdminPassword(generatedPassword)
	}
	for platform, userID := range cfg.Robots.ServiceAccountUserIDs() {
		user, userErr := db.GetRBACUserByID(userID)
		if userErr != nil || !user.Enabled {
			return nil, fmt.Errorf("robots.%s.auth.service_user_id must point to an enabled RBAC user", platform)
		}
	}

	auditSvc := audit.NewService(db, cfg, log.Logger)
	audit.RegisterConversationCreateHook(auditSvc)
	auditSvc.PurgeExpired()
	audit.StartRetentionLoop(auditSvc, log.Logger)
	if err := db.PurgeWorkflowPackageLifecycle(time.Now().UTC()); err != nil {
		log.Logger.Warn("cleanup expired workflow package records failed", zap.Error(err))
	}
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := db.PurgeWorkflowPackageLifecycle(time.Now().UTC()); err != nil {
				log.Logger.Warn("cleanup expired workflow package records failed", zap.Error(err))
			}
		}
	}()

	monitorRetention := monitor.NewService(db, cfg, log.Logger)
	monitorRetention.PurgeExpired()
	monitor.StartRetentionLoop(monitorRetention, log.Logger)

	if err := handler.NewHITLManager(db, log.Logger).EnsureSchema(); err != nil {
		log.Logger.Warn("initializing HITL table failed", zap.Error(err))
	}
	hitlRetention := hitl.NewService(db, cfg, log.Logger)
	hitlRetention.PurgeExpired()
	hitl.StartRetentionLoop(hitlRetention, log.Logger)

	// create MCP server (with database persistence)
	mcpServer := mcp.NewServerWithStorage(log.Logger, db)
	mcpServer.SetToolAuthorizer(mcpToolAuthorizer(db))
	mcpServer.SetToolGuard(toolGuard)
	mcpServer.ConfigureHTTPToolCallTimeoutFromAgentMinutes(cfg.Agent.ToolTimeoutMinutes)
	mcpServer.ConfigureToolWaitTimeoutSeconds(cfg.Agent.ToolWaitTimeoutSeconds)
	mcpServer.ConfigureToolResultMaxBytes(cfg.MultiAgent.EinoMiddleware.ReductionMaxLengthForTruncEffective())
	mcpServer.ConfigureToolResultSpillRoot(cfg.MultiAgent.EinoMiddleware.ReductionRootDir)

	// create secure tool executor
	executor := security.NewExecutor(&cfg.Security, mcpServer, log.Logger)
	executor.SetShellNoOutputTimeoutSeconds(cfg.Agent.ShellNoOutputTimeoutSeconds)
	executor.SetToolOutputMaxBytes(cfg.MultiAgent.EinoMiddleware.ReductionMaxLengthForTruncEffective())
	executor.SetToolOutputSpillRoot(cfg.MultiAgent.EinoMiddleware.ReductionRootDir)

	// register tools
	executor.RegisterTools(mcpServer)

	// register vulnerability recording tools
	registerVulnerabilityTools(mcpServer, db, log.Logger)
	registerAssetTools(mcpServer, db, log.Logger)
	registerProjectFactTools(mcpServer, db, cfg, log.Logger)
	registerVisionTools(mcpServer, cfg, log.Logger)

	// create external MCP manager (using same storage as internal MCP server)
	externalMCPMgr := mcp.NewExternalMCPManagerWithStorage(log.Logger, db)
	externalMCPMgr.SetToolAuthorizer(externalMCPToolAuthorizer())
	externalMCPMgr.SetToolGuard(toolGuard)
	externalMCPMgr.ConfigureToolWaitTimeoutSeconds(cfg.Agent.ToolWaitTimeoutSeconds)
	externalMCPMgr.ConfigureToolResultMaxBytes(cfg.MultiAgent.EinoMiddleware.ReductionMaxLengthForTruncEffective())
	externalMCPMgr.ConfigureToolResultSpillRoot(cfg.MultiAgent.EinoMiddleware.ReductionRootDir)
	externalMCPMgr.ConfigureResilience(mcp.ExternalMCPResilienceConfig{
		MaxConcurrentPerServer:  cfg.Agent.ExternalMCPMaxConcurrentPerServer,
		MaxConcurrentTotal:      cfg.Agent.ExternalMCPMaxConcurrentTotal,
		CircuitFailureThreshold: cfg.Agent.ExternalMCPCircuitFailureThreshold,
		CircuitCooldown:         time.Duration(cfg.Agent.ExternalMCPCircuitCooldownSeconds) * time.Second,
	})
	mcp.RegisterExecutionControlTools(mcpServer, externalMCPMgr)
	if cfg.ExternalMCP.Servers != nil {
		externalMCPMgr.LoadConfigs(&cfg.ExternalMCP)
		// start all enabled external MCP clients
		externalMCPMgr.StartAllEnabled()
	}

	execReconciler := monitor.NewExecutionReconciler(db, mcpServer, externalMCPMgr, log.Logger)
	execReconciler.ReconcileOnStartup()
	monitor.StartStaleRunningReconcileLoop(execReconciler, log.Logger)

	// createAgent
	maxIterations := cfg.Agent.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 30 // default value
	}
	agent := agent.NewAgent(&cfg.OpenAI, &cfg.Agent, mcpServer, externalMCPMgr, log.Logger, maxIterations)
	agent.UpdateToolDescriptionMode(cfg.Security.ToolDescriptionMode)

	// initialize knowledge base module (if enabled)
	var knowledgeManager *knowledge.Manager
	var knowledgeRetriever *knowledge.Retriever
	var knowledgeIndexer *knowledge.Indexer
	var knowledgeHandler *handler.KnowledgeHandler

	var knowledgeDBConn *database.DB
	log.Logger.Debug("checkknowledge base configuration", zap.Bool("enabled", cfg.Knowledge.Enabled))
	if cfg.Knowledge.Enabled {
		// OKknowledge base database path
		knowledgeDBPath := cfg.Database.KnowledgeDBPath
		var knowledgeDB *sql.DB

		if knowledgeDBPath != "" {
			// use a separate knowledge base database
			// ensure directory exists
			if err := os.MkdirAll(filepath.Dir(knowledgeDBPath), 0755); err != nil {
				return nil, fmt.Errorf("create knowledge basedatabasedirectoryfailed: %w", err)
			}

			var err error
			knowledgeDBConn, err = database.NewKnowledgeDB(knowledgeDBPath, log.Logger)
			if err != nil {
				return nil, fmt.Errorf("initializing knowledge base database failed: %w", err)
			}
			knowledgeDB = knowledgeDBConn.DB
			log.Logger.Info("using separate knowledge base database", zap.String("path", knowledgeDBPath))
		} else {
			// backward compatible: use session database
			knowledgeDB = db.DB
			log.Logger.Info("using session database for knowledge base data (recommended: configure knowledge_db_path to separate data)")
		}

		// create knowledge base manager
		knowledgeManager = knowledge.NewManager(knowledgeDB, cfg.Knowledge.BasePath, log.Logger)

		// create embedder
		// use OpenAI config's API Key (if not specified in knowledge base configuration)
		if cfg.Knowledge.Embedding.APIKey == "" {
			cfg.Knowledge.Embedding.APIKey = cfg.OpenAI.APIKey
		}
		if cfg.Knowledge.Embedding.BaseURL == "" {
			cfg.Knowledge.Embedding.BaseURL = cfg.OpenAI.BaseURL
		}

		embedder, err := knowledge.NewEmbedder(context.Background(), &cfg.Knowledge, &cfg.OpenAI, log.Logger)
		if err != nil {
			return nil, fmt.Errorf("initializing knowledge base embedder failed: %w", err)
		}

		// create retriever (Eino MultiQuery + reranking pipeline)
		retrievalConfig := knowledge.RetrievalConfigFromYAML(cfg.Knowledge.Retrieval)
		knowledgeRetriever = knowledge.NewRetriever(knowledgeDB, embedder, retrievalConfig, log.Logger)
		if err := knowledge.WireRetrieverPipeline(context.Background(), knowledgeRetriever, &cfg.OpenAI); err != nil {
			return nil, fmt.Errorf("initializing knowledge base retrieval pipeline failed: %w", err)
		}

		// create indexer (Eino Compose chain)
		knowledgeIndexer, err = knowledge.NewIndexer(context.Background(), knowledgeDB, embedder, log.Logger, &cfg.Knowledge)
		if err != nil {
			return nil, fmt.Errorf("initializing knowledge base indexer failed: %w", err)
		}

		// register knowledge retrieval tools to MCP server
		knowledge.RegisterKnowledgeTool(mcpServer, knowledgeRetriever, knowledgeManager, log.Logger)

		// create knowledge base API handler
		knowledgeHandler = handler.NewKnowledgeHandler(knowledgeManager, knowledgeRetriever, knowledgeIndexer, db, log.Logger)
		knowledgeHandler.SetAudit(auditSvc)
		log.Logger.Info("knowledge base module initialization completed", zap.Bool("handler_created", knowledgeHandler != nil))

		// scan knowledge base and build index (async)
		go func() {
			itemsToIndex, err := knowledgeManager.ScanKnowledgeBase()
			if err != nil {
				log.Logger.Warn("scanning knowledge base failed", zap.Error(err))
				return
			}

			// check whether index already exists
			hasIndex, err := knowledgeIndexer.HasIndex()
			if err != nil {
				log.Logger.Warn("check index status failed", zap.Error(err))
				return
			}

			if hasIndex {
				// if index already exists, only index newly added or updated items
				if len(itemsToIndex) > 0 {
					log.Logger.Info("existing knowledge base index detected, starting incremental indexing", zap.Int("count", len(itemsToIndex)))
					ctx := context.Background()
					consecutiveFailures := 0
					var firstFailureItemID string
					var firstFailureError error
					failedCount := 0

					for _, itemID := range itemsToIndex {
						if err := knowledgeIndexer.IndexItem(ctx, itemID); err != nil {
							failedCount++
							consecutiveFailures++

							if consecutiveFailures == 1 {
								firstFailureItemID = itemID
								firstFailureError = err
								log.Logger.Warn("indexing knowledge item failed", zap.String("itemId", itemID), zap.Error(err))
							}

							// if 2 consecutive failures, immediately stop incremental indexing
							if consecutiveFailures >= 2 {
								log.Logger.Error("too many consecutive index failures, stopping incremental indexing immediately",
									zap.Int("consecutiveFailures", consecutiveFailures),
									zap.Int("totalItems", len(itemsToIndex)),
									zap.String("firstFailureItemId", firstFailureItemID),
									zap.Error(firstFailureError),
								)
								break
							}
							continue
						}

						// reset consecutive failure count on success
						if consecutiveFailures > 0 {
							consecutiveFailures = 0
							firstFailureItemID = ""
							firstFailureError = nil
						}
					}
					log.Logger.Info("incremental indexing completed", zap.Int("totalItems", len(itemsToIndex)), zap.Int("failedCount", failedCount))
				} else {
					log.Logger.Info("existing knowledge base index detected, no new or updated items to index")
				}
				return
			}

			// cold start: only build index for knowledge items without vectors (consistent with IndexMissing semantics)
			log.Logger.Info("no knowledge base index detected, starting automatic index build")
			ctx := context.Background()
			if err := knowledgeIndexer.IndexMissing(ctx); err != nil {
				log.Logger.Warn("auto-build knowledge base indexing failed", zap.Error(err))
			}
		}()
	}

	// config file path must be passed in from the entry point (consistent with flag -config). Do not use os.Args[1] or ./kestrel --https will treat --https as the path.
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		configPath = "config.yaml"
	}

	skillsDir := skillpackage.SkillsRootFromConfig(cfg.SkillsDir, configPath)
	log.Logger.Debug("Skills directory (Eino ADK skill middleware + Web management API)", zap.String("skillsDir", skillsDir))
	configDir := filepath.Dir(configPath)
	plantaskRel := strings.TrimSpace(cfg.MultiAgent.EinoMiddleware.PlantaskRelDir)
	if plantaskRel == "" {
		plantaskRel = ".eino/plantask"
	}
	plantaskBase := filepath.Join(skillsDir, plantaskRel)
	// Match eino_adk_run_loop: checkpoint_dir is used as configured (relative to process CWD when not absolute).
	checkpointBase := strings.TrimSpace(cfg.MultiAgent.EinoMiddleware.CheckpointDir)
	reductionRoot := strings.TrimSpace(cfg.MultiAgent.EinoMiddleware.ReductionRootDir)
	workspaceRoot := strings.TrimSpace(cfg.Agent.WorkspaceRootDir)
	db.SetEinoConversationDirs(plantaskBase, checkpointBase, reductionRoot, workspaceRoot)

	// runtime garbage cleanup: root directory reuses the same set of values already parsed above,
	// avoid re-deriving paths in the storage package causing inconsistency between 'cleanup directory' and 'actual write directory'.
	workspaceRootDir := strings.TrimSpace(workspaceRoot)
	if workspaceRootDir == "" {
		workspaceRootDir = filepath.Join("tmp", "workspace")
	}
	reductionRootDir := strings.TrimSpace(reductionRoot)
	if reductionRootDir == "" {
		reductionRootDir = filepath.Join("tmp", "reduction")
	}
	diagnosticLogDir := strings.TrimSpace(cfg.Log.DiagnosticDir)
	if diagnosticLogDir == "" {
		diagnosticLogDir = "log"
	}
	// chat_uploads and tmp/c2 are both fixed paths relative to the process working directory
	// (see handler.chatUploadsRootDirName and c2.NewManager in app/c2_lifecycle.go).
	chatUploadsRoot := "chat_uploads"
	c2Root := filepath.Join("tmp", "c2")
	// let DeleteConversation also delete uploaded attachments: its chat_upload_artifacts row has been
	// ON DELETE CASCADE cleanup; previously disk files would remain permanently.
	db.SetChatUploadsDir(chatUploadsRoot)
	storageCleaner := storage.NewCleaner(storage.Options{
		Config: cfg,
		Paths: storage.Paths{
			Workspace:            workspaceRootDir,
			Reduction:            reductionRootDir,
			ConversationArtifact: db.ConversationArtifactsBaseDir(),
			Plantask:             plantaskBase,
			C2:                   c2Root,
			ChatUploads:          chatUploadsRoot,
			WorkflowCheckpoints:  filepath.Join(filepath.Dir(dbPath), "workflow-checkpoints"),
			DiagnosticLogs:       diagnosticLogDir,
		},
		Activity: db,
		Logger:   log.Logger,
	})
	storageService := storage.NewService(storageCleaner, cfg, log.Logger)
	storage.StartRetentionLoop(storageService, log.Logger)
	storageHandler := handler.NewStorageHandler(storageCleaner, cfg, log.Logger)
	storageHandler.SetAudit(auditSvc)

	agent.SetPromptBaseDir(configDir)

	agentsDir := cfg.AgentsDir
	if agentsDir == "" {
		agentsDir = "agents"
	}
	if !filepath.IsAbs(agentsDir) {
		agentsDir = filepath.Join(configDir, agentsDir)
	}
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		log.Logger.Warn("create agents directory failed", zap.String("path", agentsDir), zap.Error(err))
	}
	markdownAgentsHandler := handler.NewMarkdownAgentsHandler(agentsDir)
	markdownAgentsHandler.SetAudit(auditSvc)
	log.Logger.Debug("multi-agent Markdown sub-Agent directory", zap.String("agentsDir", agentsDir))

	// create handlers
	agentHandler := handler.NewAgentHandler(agent, db, cfg, log.Logger)
	agentHandler.SetAudit(auditSvc)
	agentHandler.SetAgentsMarkdownDir(agentsDir)
	// if knowledge base is enabled, set knowledge base manager on AgentHandler to record retrieval logs
	if knowledgeManager != nil {
		agentHandler.SetKnowledgeManager(knowledgeManager)
	}
	monitorHandler := handler.NewMonitorHandler(mcpServer, executor, db, log.Logger)
	monitorHandler.SetAudit(auditSvc)
	monitorHandler.SetMonitorRetention(monitorRetention)
	monitorHandler.SetExternalMCPManager(externalMCPMgr) // set external MCP manager to retrieve external MCP execution records
	monitorHandler.SetTaskManager(agentHandler.TaskManager())
	monitorHandler.SetAgentHandler(agentHandler)
	notificationHandler := handler.NewNotificationHandler(db, agentHandler, log.Logger)
	authHandler := handler.NewAuthHandler(authManager, cfg, configPath, log.Logger)
	authHandler.SetAudit(auditSvc)
	attackChainHandler := handler.NewAttackChainHandler(db, &cfg.OpenAI, log.Logger)
	vulnerabilityHandler := handler.NewVulnerabilityHandler(db, log.Logger)
	assetHandler := handler.NewAssetHandler(db, log.Logger)
	projectHandler := handler.NewProjectHandler(db, log.Logger)
	projectHandler.SetAuthManager(authManager)
	rbacHandler := handler.NewRBACHandler(db, log.Logger)
	rbacHandler.SetAudit(auditSvc)
	rbacHandler.SetAuthManager(authManager)
	workflowHandler := handler.NewWorkflowHandler(db, log.Logger)
	workflowHandler.SetAudit(auditSvc)
	workflowHandler.SetRuntime(agent, cfg)
	vulnerabilityHandler.SetAudit(auditSvc)
	vulnerabilityHandler.SetWebhookConfig(cfg.Security.WebhookURL, cfg.Security.WebhookMinSeverity)
	webshellHandler := handler.NewWebShellHandler(log.Logger, db)
	webshellHandler.SetAudit(auditSvc)
	chatUploadsHandler := handler.NewChatUploadsHandler(log.Logger, db)
	chatUploadsHandler.SetAudit(auditSvc)
	registerWebshellTools(mcpServer, db, webshellHandler, log.Logger)
	registerWebshellManagementTools(mcpServer, db, webshellHandler, log.Logger)
	configHandler := handler.NewConfigHandler(configPath, cfg, mcpServer, executor, agent, attackChainHandler, externalMCPMgr, log.Logger)
	configHandler.SetDB(db)
	configHandler.SetToolGuard(toolGuard)
	configHandler.SetAudit(auditSvc)
	agentHandler.SetHitlToolWhitelistSaver(configHandler)
	agentHandler.SetHitlAuditStrategySaver(configHandler)
	agentHandler.SetHitlDefaultReviewerSaver(configHandler)
	externalMCPHandler := handler.NewExternalMCPHandler(externalMCPMgr, cfg, configPath, log.Logger)
	externalMCPHandler.SetAudit(auditSvc)
	roleHandler := handler.NewRoleHandler(cfg, configPath, log.Logger)
	roleHandler.SetDB(db)
	roleHandler.SetAudit(auditSvc)
	skillsHandler := handler.NewSkillsHandler(cfg, configPath, log.Logger)
	skillsHandler.SetAudit(auditSvc)
	fofaHandler := handler.NewFofaHandler(cfg, log.Logger)
	terminalHandler := handler.NewTerminalHandler(log.Logger)
	terminalHandler.SetAudit(auditSvc)
	if db != nil {
		skillsHandler.SetDB(db) // set database connection to retrieve call statistics
	}

	// ============================================================================
	// initialize C2 module (can be disabled by config to save local deployment resources)
	// ============================================================================
	c2Manager, c2Watchdog, watchdogCancel := setupC2Runtime(cfg, db, agentHandler, log.Logger)
	if c2Manager != nil {
		registerC2Tools(mcpServer, c2Manager, log.Logger, cfg.Server.Port)
	}
	c2Handler := handler.NewC2Handler(c2Manager, log.Logger)
	c2Handler.SetAudit(auditSvc)

	// createOpenAPI handler
	conversationHandler := handler.NewConversationHandler(db, log.Logger)
	conversationHandler.SetAudit(auditSvc)
	conversationHandler.SetTaskStopper(agentHandler)
	conversationHandler.SetTaskStateProvider(agentHandler)
	auditHandler := handler.NewAuditHandler(db, auditSvc, log.Logger)
	robotHandler := handler.NewRobotHandler(cfg, db, agentHandler, log.Logger)
	robotHandler.SetAudit(auditSvc)
	db.SetVulnerabilityCreatedHook(robotHandler.NotifyNewVulnerability)
	openAPIHandler := handler.NewOpenAPIHandler(db, log.Logger, conversationHandler, agentHandler)

	// create App instance (some fields populated later)
	app := &App{
		config:             cfg,
		logger:             log,
		router:             router,
		mcpServer:          mcpServer,
		externalMCPMgr:     externalMCPMgr,
		agent:              agent,
		executor:           executor,
		db:                 db,
		knowledgeDB:        knowledgeDBConn,
		auth:               authManager,
		knowledgeManager:   knowledgeManager,
		knowledgeRetriever: knowledgeRetriever,
		knowledgeIndexer:   knowledgeIndexer,
		knowledgeHandler:   knowledgeHandler,
		agentHandler:       agentHandler,
		robotHandler:       robotHandler,
		c2Manager:          c2Manager,
		c2Watchdog:         c2Watchdog,
		c2WatchdogCancel:   watchdogCancel,
		c2Handler:          c2Handler,
		storageHandler:     storageHandler,
		auditSvc:           auditSvc,
	}
	// Feishu/DingTalk long-connection (no public internet required); started in background when enabled; restarted via RestartRobotConnections when config is applied from frontend
	app.startRobotConnections()
	alertCtx, alertCancel := context.WithCancel(context.Background())
	app.alertCancel = alertCancel
	go robotHandler.RunVulnerabilityAlertWorker(alertCtx)

	// set vulnerability tool registrar (built-in tools, must be set)
	vulnerabilityRegistrar := func() error {
		registerVulnerabilityTools(mcpServer, db, log.Logger)
		registerAssetTools(mcpServer, db, log.Logger)
		registerProjectFactTools(mcpServer, db, cfg, log.Logger)
		registerVisionTools(mcpServer, cfg, log.Logger)
		return nil
	}
	configHandler.SetVulnerabilityToolRegistrar(vulnerabilityRegistrar)

	// set WebShell tool registrar (re-registers on ApplyConfig)
	webshellRegistrar := func() error {
		registerWebshellTools(mcpServer, db, webshellHandler, log.Logger)
		registerWebshellManagementTools(mcpServer, db, webshellHandler, log.Logger)
		return nil
	}
	configHandler.SetWebshellToolRegistrar(webshellRegistrar)

	// Skills are provided by Eino ADK skill middleware (multi-agent); MCP-form skill tools are not registered here
	configHandler.SetSkillsToolRegistrar(func() error { return nil })

	handler.RegisterBatchTaskMCPTools(mcpServer, agentHandler, log.Logger)
	batchTaskToolRegistrar := func() error {
		handler.RegisterBatchTaskMCPTools(mcpServer, agentHandler, log.Logger)
		return nil
	}
	configHandler.SetBatchTaskToolRegistrar(batchTaskToolRegistrar)

	// set knowledge base initializer (for dynamic initialization, must be set after App creation)
	configHandler.SetKnowledgeInitializer(func() (*handler.KnowledgeHandler, error) {
		knowledgeHandler, err := initializeKnowledge(cfg, db, knowledgeDBConn, mcpServer, agentHandler, app, log.Logger)
		if err != nil {
			return nil, err
		}

		// after dynamic initialization, set knowledge base tool registrar and retriever updater
		// so tools can be re-registered on subsequent ApplyConfig calls
		if app.knowledgeRetriever != nil && app.knowledgeManager != nil {
			// create closure capturing references to knowledgeRetriever and knowledgeManager
			registrar := func() error {
				knowledge.RegisterKnowledgeTool(mcpServer, app.knowledgeRetriever, app.knowledgeManager, log.Logger)
				return nil
			}
			configHandler.SetKnowledgeToolRegistrar(registrar)
			// set retriever updater to update retriever config on ApplyConfig
			configHandler.SetRetrieverUpdater(app.knowledgeRetriever)
			log.Logger.Info("knowledge base tool registrar and retriever updater set after dynamic initialization")
		}

		return knowledgeHandler, nil
	})

	// if knowledge base is enabled, set knowledge base tool registrar and retriever updater
	if cfg.Knowledge.Enabled && knowledgeRetriever != nil && knowledgeManager != nil {
		// create closure capturing references to knowledgeRetriever and knowledgeManager
		registrar := func() error {
			knowledge.RegisterKnowledgeTool(mcpServer, knowledgeRetriever, knowledgeManager, log.Logger)
			return nil
		}
		configHandler.SetKnowledgeToolRegistrar(registrar)
		// set retriever updater to update retriever config on ApplyConfig
		configHandler.SetRetrieverUpdater(knowledgeRetriever)
	}

	// set bot connection restarter so DingTalk/Feishu/WeChat new config takes effect without restarting the service after frontend applies config
	configHandler.SetRobotRestarter(app)

	wechatRobotHandler := handler.NewWechatRobotHandler(cfg, configHandler, log.Logger)

	configHandler.SetC2Runtime(app)
	configHandler.SetC2ToolRegistrar(func() error {
		if app.config.C2.EnabledEffective() && app.c2Manager != nil {
			registerC2Tools(mcpServer, app.c2Manager, log.Logger, app.config.Server.Port)
		}
		return nil
	})

	// set routes (using App instance to dynamically retrieve handlers)
	setupRoutes(
		router,
		authHandler,
		agentHandler,
		monitorHandler,
		notificationHandler,
		conversationHandler,
		robotHandler,
		wechatRobotHandler,
		configHandler,
		externalMCPHandler,
		attackChainHandler,
		app, // pass App instance to dynamically retrieve knowledgeHandler
		vulnerabilityHandler,
		assetHandler,
		projectHandler,
		workflowHandler,
		webshellHandler,
		chatUploadsHandler,
		roleHandler,
		skillsHandler,
		markdownAgentsHandler,
		fofaHandler,
		terminalHandler,
		app.c2Handler,
		auditHandler,
		auditSvc,
		rbacHandler,
		mcpServer,
		authManager,
		openAPIHandler,
	)

	return app, nil

}

// mcpHandlerWithAuth forwards to MCP handler after auth check; validates request header if auth_header is configured, otherwise passes through
func (a *App) mcpHandlerWithAuth(w http.ResponseWriter, r *http.Request) {
	cfg := a.config.MCP
	if authHeader := strings.TrimSpace(r.Header.Get("Authorization")); len(authHeader) > 7 && strings.EqualFold(authHeader[:7], "Bearer ") {
		if session, ok := a.auth.ValidateToken(strings.TrimSpace(authHeader[7:])); ok && session.Permissions["mcp:execute"] {
			principal := authctx.NewPrincipalWithScopes(session.UserID, session.Username, session.Scope, session.Permissions, session.PermissionScopes)
			a.mcpServer.HandleHTTP(w, r.WithContext(authctx.WithPrincipal(r.Context(), principal)))
			return
		}
	}
	if !cfg.AllowGlobalAccess || strings.TrimSpace(cfg.AuthHeader) == "" || strings.TrimSpace(cfg.AuthHeaderValue) == "" {
		http.Error(w, "use an authorized user bearer token; global MCP service access is disabled", http.StatusUnauthorized)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(cfg.AuthHeader)), []byte(cfg.AuthHeaderValue)) != 1 {
		a.logger.Logger.Debug("MCP auth failed: header missing or value mismatch", zap.String("header", cfg.AuthHeader))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		return
	}
	permissions := make(map[string]bool, len(security.PermissionCatalog))
	for permission := range security.PermissionCatalog {
		permissions[permission] = true
	}
	principal := authctx.NewPrincipal("service:mcp", "mcp-service", database.RBACScopeAll, permissions)
	r = r.WithContext(authctx.WithPrincipal(r.Context(), principal))
	a.mcpServer.HandleHTTP(w, r)
}

// Run starts the application (backward compatible, no graceful shutdown)
func (a *App) Run() error {
	return a.RunWithContext(context.Background())
}

// RunWithContext starts the application with graceful shutdown via context cancellation
func (a *App) RunWithContext(ctx context.Context) error {
	// start MCP server (if enabled)
	var mcpServer *http.Server
	if a.config.MCP.Enabled {
		mcpAddr := fmt.Sprintf("%s:%d", a.config.MCP.Host, a.config.MCP.Port)
		a.logger.Info("starting MCP server", zap.String("address", mcpAddr))

		mux := http.NewServeMux()
		mux.HandleFunc("/mcp", a.mcpHandlerWithAuth)

		mcpServer = &http.Server{Addr: mcpAddr, Handler: mux}
		go func() {
			if err := mcpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				a.logger.Error("MCP server startup failed", zap.Error(err))
			}
		}()
	}

	// start main server (optional HTTPS + HTTP/2, see config server.tls_*)
	addr := fmt.Sprintf("%s:%d", a.config.Server.Host, a.config.Server.Port)
	tlsMode, tlsConf, certFile, keyFile, tlsErr := prepareMainServerTLS(&a.config.Server)
	if tlsErr != nil {
		return tlsErr
	}

	srv := &http.Server{Addr: addr, Handler: a.router}
	var mainMux *mainServerMux
	httpRedirect := config.ServerHTTPRedirectEnabled(&a.config.Server)
	if tlsMode != mainTLSOff {
		srv.TLSConfig = tlsConf
		if err := http2.ConfigureServer(srv, &http2.Server{}); err != nil {
			return fmt.Errorf("main server HTTP/2 config failed: %w", err)
		}
		switch tlsMode {
		case mainTLSFromFiles:
			a.logger.Debug("starting HTTPS main server (HTTP/2 negotiation enabled)",
				zap.String("address", addr),
				zap.String("cert", certFile),
			)
		case mainTLSInMemorySelfSigned:
			a.logger.Debug("starting HTTPS main server (in-memory self-signed cert, test only; HTTP/2 negotiation enabled)",
				zap.String("address", addr),
			)
		}
		if httpRedirect {
			a.logger.Debug("enabled HTTP→HTTPS auto-redirect (same port sniff split)", zap.String("address", addr))
		}
	} else {
		a.logger.Debug("starting HTTP main server", zap.String("address", addr))
	}

	// listen for context cancellation to gracefully shut down HTTP server
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if mainMux != nil {
			if err := mainMux.Shutdown(shutdownCtx); err != nil {
				a.logger.Error("HTTP/HTTPS split server close failed", zap.Error(err))
			}
		} else if err := srv.Shutdown(shutdownCtx); err != nil {
			a.logger.Error("HTTP server close failed", zap.Error(err))
		}
		if mcpServer != nil {
			if err := mcpServer.Shutdown(shutdownCtx); err != nil {
				a.logger.Error("MCP server close failed", zap.Error(err))
			}
		}
	}()

	var err error
	switch {
	case tlsMode != mainTLSOff && httpRedirect:
		var tlsConfReady *tls.Config
		tlsConfReady, err = ensureMainTLSConfigCerts(tlsMode, tlsConf, certFile, keyFile)
		if err != nil {
			return fmt.Errorf("loading TLS certificate: %w", err)
		}
		srv.TLSConfig = tlsConfReady
		var ln net.Listener
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		mainMux = newMainServerMux(ln, srv, portFromListenAddr(addr), a.logger.Logger)
		err = mainMux.Serve()
	case tlsMode == mainTLSOff:
		err = srv.ListenAndServe()
	case tlsMode == mainTLSFromFiles:
		err = srv.ListenAndServeTLS(certFile, keyFile)
	case tlsMode == mainTLSInMemorySelfSigned:
		var ln net.Listener
		ln, err = tls.Listen("tcp", addr, srv.TLSConfig)
		if err == nil {
			err = srv.Serve(ln)
		}
	default:
		err = srv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown closes the application
func (a *App) Shutdown() {
	if a.agentHandler != nil {
		a.agentHandler.ShutdownTasks()
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = einoobserve.ShutdownOtel(shutdownCtx)
	shutdownCancel()
	if a.alertCancel != nil {
		a.alertCancel()
		a.alertCancel = nil
	}

	// stop DingTalk/Feishu long-connections
	a.robotMu.Lock()
	if a.dingCancel != nil {
		a.dingCancel()
		a.dingCancel = nil
	}
	if a.larkCancel != nil {
		a.larkCancel()
		a.larkCancel = nil
	}
	a.robotMu.Unlock()

	a.shutdownC2()

	// stop all external MCP clients
	if a.externalMCPMgr != nil {
		a.externalMCPMgr.StopAll()
	}

	// close knowledge base database connection (if using a separate database)
	if a.knowledgeDB != nil {
		if err := a.knowledgeDB.Close(); err != nil {
			a.logger.Logger.Warn("close knowledge base database connection failed", zap.Error(err))
		}
	}

	// close main database connection
	if a.db != nil {
		if err := a.db.Close(); err != nil {
			a.logger.Logger.Warn("close main database connection failed", zap.Error(err))
		}
	}
}

// startRobotConnections starts DingTalk/Feishu long-connections based on current config (does not close existing connections, for initial startup only)
func (a *App) startRobotConnections() {
	a.robotMu.Lock()
	defer a.robotMu.Unlock()
	cfg := a.config
	if cfg.Robots.Lark.Enabled && cfg.Robots.Lark.AppID != "" && cfg.Robots.Lark.AppSecret != "" {
		ctx, cancel := context.WithCancel(context.Background())
		a.larkCancel = cancel
		go robot.StartLark(ctx, cfg.Robots, a.robotHandler, a.logger.Logger)
	}
	if cfg.Robots.Dingtalk.Enabled && cfg.Robots.Dingtalk.ClientID != "" && cfg.Robots.Dingtalk.ClientSecret != "" {
		ctx, cancel := context.WithCancel(context.Background())
		a.dingCancel = cancel
		go robot.StartDing(ctx, cfg.Robots, a.robotHandler, a.logger.Logger)
	}
	if cfg.Robots.Wechat.Enabled && cfg.Robots.Wechat.BotToken != "" {
		ctx, cancel := context.WithCancel(context.Background())
		a.wechatCancel = cancel
		go robot.StartWechat(ctx, cfg.Robots, a.robotHandler, cfg.Version, a.logger.Logger)
	}
	if cfg.Robots.Telegram.Enabled && strings.TrimSpace(cfg.Robots.Telegram.BotToken) != "" {
		ctx, cancel := context.WithCancel(context.Background())
		a.telegramCancel = cancel
		go robot.StartTelegram(ctx, cfg.Robots, a.robotHandler, a.logger.Logger)
	}
	if cfg.Robots.Slack.Enabled && strings.TrimSpace(cfg.Robots.Slack.BotToken) != "" && strings.TrimSpace(cfg.Robots.Slack.AppToken) != "" {
		ctx, cancel := context.WithCancel(context.Background())
		a.slackCancel = cancel
		go robot.StartSlack(ctx, cfg.Robots, a.robotHandler, a.logger.Logger)
	}
	if cfg.Robots.Discord.Enabled && strings.TrimSpace(cfg.Robots.Discord.BotToken) != "" {
		ctx, cancel := context.WithCancel(context.Background())
		a.discordCancel = cancel
		go robot.StartDiscord(ctx, cfg.Robots, a.robotHandler, a.logger.Logger)
	}
	if cfg.Robots.QQ.Enabled && strings.TrimSpace(cfg.Robots.QQ.AppID) != "" && strings.TrimSpace(cfg.Robots.QQ.ClientSecret) != "" {
		ctx, cancel := context.WithCancel(context.Background())
		a.qqCancel = cancel
		go robot.StartQQ(ctx, cfg.Robots, a.robotHandler, a.logger.Logger)
	}
}

// RestartRobotConnections restarts DingTalk/Feishu/WeChat long-connections to take effect immediately after frontend config apply (implements handler.RobotRestarter)
func (a *App) RestartRobotConnections() {
	a.robotMu.Lock()
	if a.dingCancel != nil {
		a.dingCancel()
		a.dingCancel = nil
	}
	if a.larkCancel != nil {
		a.larkCancel()
		a.larkCancel = nil
	}
	if a.wechatCancel != nil {
		a.wechatCancel()
		a.wechatCancel = nil
	}
	if a.telegramCancel != nil {
		a.telegramCancel()
		a.telegramCancel = nil
	}
	if a.slackCancel != nil {
		a.slackCancel()
		a.slackCancel = nil
	}
	if a.discordCancel != nil {
		a.discordCancel()
		a.discordCancel = nil
	}
	if a.qqCancel != nil {
		a.qqCancel()
		a.qqCancel = nil
	}
	a.robotMu.Unlock()
	// give old goroutine time to exit
	time.Sleep(200 * time.Millisecond)
	a.startRobotConnections()
}

// DB returns the primary database handle.
func (a *App) DB() *database.DB {
	return a.db
}

// Auth returns the authentication manager.
func (a *App) Auth() *security.AuthManager {
	return a.auth
}
