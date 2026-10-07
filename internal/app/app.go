package app

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	"kestrel/internal/mcp/builtin"
	"kestrel/internal/monitor"
	"kestrel/internal/multiagent"
	"kestrel/internal/robot"
	"kestrel/internal/security"
	"kestrel/internal/skillpackage"
	"kestrel/internal/storage"
	"kestrel/internal/toolguard"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
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

	// CORS middleware
	router.Use(corsMiddleware(cfg.Server.CORSAllowedOrigins))

	// initialize database
	dbPath := cfg.Database.Path
	if dbPath == "" {
		dbPath = "data/conversations.db"
	}

	// ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("createdatabasedirectoryfailed: %w", err)
	}

	db, err := database.NewDB(dbPath, log.Logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// auth manager (RBAC mounted after database initialization)
	authManager := security.NewAuthManager(cfg.Auth.SessionDurationHours)
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
				log.Logger.Warn("checkindexstatusfailed", zap.Error(err))
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
		log.Logger.Warn("create agents directoryfailed", zap.String("path", agentsDir), zap.Error(err))
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
	rbacHandler := handler.NewRBACHandler(db, log.Logger)
	rbacHandler.SetAudit(auditSvc)
	rbacHandler.SetAuthManager(authManager)
	workflowHandler := handler.NewWorkflowHandler(db, log.Logger)
	workflowHandler.SetAudit(auditSvc)
	workflowHandler.SetRuntime(agent, cfg)
	vulnerabilityHandler.SetAudit(auditSvc)
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
		w.Write([]byte(`{"error":"unauthorized"}`))
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

// setupRoutes sets up routes
func setupRoutes(
	router *gin.Engine,
	authHandler *handler.AuthHandler,
	agentHandler *handler.AgentHandler,
	monitorHandler *handler.MonitorHandler,
	notificationHandler *handler.NotificationHandler,
	conversationHandler *handler.ConversationHandler,
	robotHandler *handler.RobotHandler,
	wechatRobotHandler *handler.WechatRobotHandler,
	configHandler *handler.ConfigHandler,
	externalMCPHandler *handler.ExternalMCPHandler,
	attackChainHandler *handler.AttackChainHandler,
	app *App, // pass App instance to dynamically retrieve knowledgeHandler
	vulnerabilityHandler *handler.VulnerabilityHandler,
	assetHandler *handler.AssetHandler,
	projectHandler *handler.ProjectHandler,
	workflowHandler *handler.WorkflowHandler,
	webshellHandler *handler.WebShellHandler,
	chatUploadsHandler *handler.ChatUploadsHandler,
	roleHandler *handler.RoleHandler,
	skillsHandler *handler.SkillsHandler,
	markdownAgentsHandler *handler.MarkdownAgentsHandler,
	fofaHandler *handler.FofaHandler,
	terminalHandler *handler.TerminalHandler,
	c2Handler *handler.C2Handler,
	auditHandler *handler.AuditHandler,
	auditSvc *audit.Service,
	rbacHandler *handler.RBACHandler,
	mcpServer *mcp.Server,
	authManager *security.AuthManager,
	openAPIHandler *handler.OpenAPIHandler,
) {
	// API routes
	api := router.Group("/api")

	// authentication routes
	authRoutes := api.Group("/auth")
	loginRL := security.NewRateLimiter(10, 1*time.Minute)
	{
		authRoutes.POST("/login", security.RateLimitMiddleware(loginRL), authHandler.Login)
		authRoutes.POST("/logout", security.AuthMiddleware(authManager), authHandler.Logout)
		authRoutes.POST("/change-password", security.AuthMiddleware(authManager), security.RequirePermission("auth:self"), authHandler.ChangePassword)
		authRoutes.GET("/validate", security.AuthMiddleware(authManager), authHandler.Validate)
		authRoutes.POST("/robot-binding-code", security.AuthMiddleware(authManager), security.RequirePermission("auth:self"), robotHandler.CreateRobotBindingCode)
		authRoutes.GET("/robot-bindings", security.AuthMiddleware(authManager), security.RequirePermission("auth:self"), robotHandler.ListMyRobotBindings)
		authRoutes.DELETE("/robot-bindings/:id", security.AuthMiddleware(authManager), security.RequirePermission("auth:self"), robotHandler.DeleteMyRobotBinding)
	}

	// bot callback (no login required, called by WeCom/DingTalk/Feishu servers)
	// add rate limit: max 60 requests per IP per minute to prevent abuse
	robotRL := security.NewRateLimiter(60, 1*time.Minute)
	robotGroup := api.Group("/robot")
	robotGroup.Use(security.RateLimitMiddleware(robotRL))
	{
		robotGroup.GET("/wecom", robotHandler.HandleWecomGET)
		robotGroup.POST("/wecom", robotHandler.HandleWecomPOST)
		robotGroup.POST("/dingtalk", robotHandler.HandleDingtalkPOST)
		robotGroup.POST("/lark", robotHandler.HandleLarkPOST)
	}

	protected := api.Group("")
	protected.Use(security.AuthMiddleware(authManager))
	protected.Use(security.RBACMiddlewareWithDenyHook(app.db, func(c *gin.Context, reason, permission string) {
		if auditSvc != nil {
			auditSvc.Record(c, audit.Entry{
				Level: "warn", Category: "rbac", Action: "access_denied", Result: "failure",
				Message: "RBAC access denied", ResourceType: "route", ResourceID: c.FullPath(),
				Detail: map[string]interface{}{"reason": reason, "permission": permission, "method": c.Request.Method},
			})
		}
	}))
	{
		protected.GET("/rbac/me", rbacHandler.Me)
		protected.GET("/rbac/metadata", rbacHandler.Metadata)
		protected.GET("/rbac/users", rbacHandler.ListUsers)
		protected.POST("/rbac/users", rbacHandler.CreateUser)
		protected.PUT("/rbac/users/:id", rbacHandler.UpdateUser)
		protected.DELETE("/rbac/users/:id", rbacHandler.DeleteUser)
		protected.GET("/rbac/roles", rbacHandler.ListRoles)
		protected.POST("/rbac/roles", rbacHandler.CreateRole)
		protected.PUT("/rbac/roles/:id", rbacHandler.UpdateRole)
		protected.DELETE("/rbac/roles/:id", rbacHandler.DeleteRole)
		protected.GET("/rbac/resource-assignments", rbacHandler.ListResourceAssignments)
		protected.GET("/rbac/resources", rbacHandler.ListAssignableResources)
		protected.POST("/rbac/resource-assignments", rbacHandler.AssignResource)
		protected.DELETE("/rbac/resource-assignments/:id", rbacHandler.DeleteResourceAssignment)

		// bot test (requires login): POST /api/robot/test, body: {"platform":"dingtalk","user_id":"test","text":"help"}, used to validate bot logic
		protected.POST("/robot/test", robotHandler.HandleRobotTest)

		// WeChat iLink QR code binding (requires login)
		protected.POST("/robot/wechat/qrcode", wechatRobotHandler.HandleWechatQRCode)
		protected.GET("/robot/wechat/qrcode/status", wechatRobotHandler.HandleWechatQRCodeStatus)
		protected.POST("/robot/wechat/qrcode/verify", wechatRobotHandler.HandleWechatVerifyCode)
		protected.GET("/robot/wechat/status", wechatRobotHandler.HandleWechatStatus)

		// Eino ADK single agent (ChatModelAgent + Runner; independent of multi_agent.enabled)
		protected.POST("/eino-agent", agentHandler.EinoSingleAgentLoop)
		protected.POST("/eino-agent/stream", agentHandler.EinoSingleAgentLoopStream)
		protected.GET("/hitl/pending", agentHandler.ListHITLPending)
		protected.GET("/hitl/logs", agentHandler.ListHITLLogs)
		protected.DELETE("/hitl/logs", agentHandler.DeleteHITLLogs)
		protected.GET("/hitl/logs/:id", agentHandler.GetHITLLog)
		protected.POST("/hitl/decision", agentHandler.DecideHITLInterrupt)
		protected.POST("/hitl/dismiss", agentHandler.DismissHITLInterrupt)
		protected.GET("/hitl/config/:conversationId", agentHandler.GetHITLConversationConfig)
		protected.PUT("/hitl/config", agentHandler.UpsertHITLConversationConfig)
		protected.GET("/hitl/tool-whitelist", agentHandler.GetHITLGlobalToolWhitelist)
		protected.PUT("/hitl/tool-whitelist", agentHandler.SetHITLGlobalToolWhitelist)
		protected.POST("/hitl/tool-whitelist", agentHandler.MergeHITLGlobalToolWhitelist)
		protected.GET("/hitl/default-config", agentHandler.GetHITLDefaultConfig)
		protected.PUT("/hitl/default-config", agentHandler.UpdateHITLDefaultConfig)
		protected.GET("/hitl/default-reviewer", agentHandler.GetHITLDefaultReviewer)
		protected.PUT("/hitl/default-reviewer", agentHandler.UpdateHITLDefaultReviewer)
		protected.GET("/hitl/audit-strategy", agentHandler.GetHITLAuditStrategy)
		protected.PUT("/hitl/audit-strategy", agentHandler.UpdateHITLAuditStrategy)
		// Agent Loop cancellation and task list
		protected.POST("/agent-loop/cancel", agentHandler.CancelAgentLoop)
		protected.GET("/agent-loop/tasks", agentHandler.ListAgentTasks)
		protected.GET("/agent-loop/task-events", agentHandler.SubscribeAgentTaskEvents)
		protected.GET("/agent-loop/tasks/completed", agentHandler.ListCompletedTasks)

		// Eino DeepAgent multi-agent (coexists with single agent; requires config.multi_agent.enabled)
		// multi-agent routes always registered; availability determined at runtime by h.config.MultiAgent.Enabled (no restart needed after config apply)
		protected.POST("/multi-agent", agentHandler.MultiAgentLoop)
		protected.POST("/multi-agent/stream", agentHandler.MultiAgentLoopStream)
		protected.GET("/multi-agent/markdown-agents", markdownAgentsHandler.ListMarkdownAgents)
		protected.GET("/multi-agent/markdown-agents/:filename", markdownAgentsHandler.GetMarkdownAgent)
		protected.POST("/multi-agent/markdown-agents", markdownAgentsHandler.CreateMarkdownAgent)
		protected.PUT("/multi-agent/markdown-agents/:filename", markdownAgentsHandler.UpdateMarkdownAgent)
		protected.DELETE("/multi-agent/markdown-agents/:filename", markdownAgentsHandler.DeleteMarkdownAgent)

		// info gathering - FOFA query (backend proxy)
		protected.POST("/fofa/search", fofaHandler.Search)
		// info gathering - natural language to FOFA syntax (requires human confirmation before query)
		protected.POST("/fofa/parse", fofaHandler.ParseNaturalLanguage)

		// asset management
		protected.GET("/assets", assetHandler.List)
		protected.GET("/assets/selection", assetHandler.Selection)
		protected.GET("/assets/stats", assetHandler.Stats)
		protected.POST("/assets/import", assetHandler.Import)
		protected.POST("/assets/scan-links", assetHandler.RecordScans)
		protected.PUT("/assets/bulk", assetHandler.BulkUpdate)
		protected.PUT("/assets/project-binding", assetHandler.UpdateProjectBinding)
		protected.POST("/assets/batch-delete", assetHandler.BatchDelete)
		protected.POST("/assets/merge", security.RequirePermission("asset:write"), assetHandler.Merge)
		protected.PUT("/assets/:id", assetHandler.Update)
		protected.DELETE("/assets/:id", assetHandler.Delete)

		// batch task management
		protected.POST("/batch-tasks", agentHandler.CreateBatchQueue)
		protected.GET("/batch-tasks", agentHandler.ListBatchQueues)
		protected.GET("/batch-tasks/:queueId", agentHandler.GetBatchQueue)
		protected.POST("/batch-tasks/:queueId/start", agentHandler.StartBatchQueue)
		protected.POST("/batch-tasks/:queueId/rerun", agentHandler.RerunBatchQueue)
		protected.POST("/batch-tasks/:queueId/pause", agentHandler.PauseBatchQueue)
		protected.PUT("/batch-tasks/:queueId/metadata", agentHandler.UpdateBatchQueueMetadata)
		protected.PUT("/batch-tasks/:queueId/schedule", agentHandler.UpdateBatchQueueSchedule)
		protected.PUT("/batch-tasks/:queueId/schedule-enabled", agentHandler.SetBatchQueueScheduleEnabled)
		protected.DELETE("/batch-tasks/:queueId", agentHandler.DeleteBatchQueue)
		protected.PUT("/batch-tasks/:queueId/tasks/:taskId", agentHandler.UpdateBatchTask)
		protected.POST("/batch-tasks/:queueId/tasks/:taskId/run", agentHandler.RunSingleBatchTask)
		protected.POST("/batch-tasks/:queueId/tasks", agentHandler.AddBatchTask)
		protected.DELETE("/batch-tasks/:queueId/tasks/:taskId", agentHandler.DeleteBatchTask)

		// conversation history
		protected.GET("/usage/tokens", conversationHandler.GetTokenUsageStats)
		protected.POST("/conversations", conversationHandler.CreateConversation)
		protected.GET("/conversations", conversationHandler.ListConversations)
		protected.GET("/conversations/:id", conversationHandler.GetConversation)
		protected.GET("/conversations/:id/token-usage", conversationHandler.GetConversationTokenUsageStats)
		protected.GET("/conversations/:id/plan-tasks", conversationHandler.GetConversationPlanTasks)
		protected.GET("/messages/:id/process-details", conversationHandler.GetMessageProcessDetails)
		protected.GET("/process-details/:id", conversationHandler.GetProcessDetail)
		protected.PUT("/conversations/:id", conversationHandler.UpdateConversation)
		protected.PUT("/conversations/:id/project", conversationHandler.SetConversationProject)
		protected.DELETE("/conversations/:id", conversationHandler.DeleteConversation)
		protected.POST("/conversations/:id/delete-turn", conversationHandler.DeleteConversationTurn)
		protected.PUT("/conversations/:id/pinned", conversationHandler.UpdateConversationPinned)

		// monitoring
		protected.GET("/monitor", monitorHandler.Monitor)
		protected.GET("/monitor/execution/:id", monitorHandler.GetExecution)
		protected.POST("/monitor/execution/:id/cancel", monitorHandler.CancelExecution)
		protected.POST("/monitor/executions/names", monitorHandler.BatchGetToolNames)
		protected.DELETE("/monitor/execution/:id", monitorHandler.DeleteExecution)
		protected.DELETE("/monitor/executions", monitorHandler.DeleteExecutions)
		protected.GET("/monitor/stats", monitorHandler.GetStats)
		protected.GET("/monitor/calls-timeline", monitorHandler.GetCallsTimeline)
		protected.GET("/notifications/summary", notificationHandler.GetSummary)
		protected.POST("/notifications/read", notificationHandler.MarkRead)

		// config management
		protected.GET("/config", configHandler.GetConfig)
		protected.GET("/tool-guard", configHandler.GetToolGuard)
		protected.PUT("/tool-guard", configHandler.UpdateToolGuard)
		protected.POST("/tool-guard/test", configHandler.TestToolGuard)
		protected.GET("/config/tools", configHandler.GetTools)
		protected.GET("/config/tools/:name/schema", configHandler.GetToolSchema)
		protected.PUT("/config", configHandler.UpdateConfig)
		protected.POST("/config/apply", configHandler.ApplyConfig)
		protected.POST("/config/test-openai", configHandler.TestOpenAI)
		protected.POST("/config/test-typesafe", configHandler.TestTypeSafe)
		protected.POST("/config/test-vision", configHandler.TestVision)
		protected.POST("/config/list-models", configHandler.ListModels)

		// system settings - terminal (execute commands, improve ops efficiency)
		protected.POST("/terminal/run", terminalHandler.RunCommand)
		protected.POST("/terminal/run/stream", terminalHandler.RunCommandStream)
		protected.GET("/terminal/ws", terminalHandler.RunCommandWS)

		// platform audit log
		protected.GET("/audit/meta", auditHandler.Meta)
		protected.GET("/audit/summary", auditHandler.Summary)
		protected.GET("/audit/logs", auditHandler.ListLogs)
		protected.GET("/audit/logs/export", auditHandler.ExportLogs)
		protected.GET("/audit/logs/:id", auditHandler.GetLog)

		// runtime storage occupancy and garbage cleanup
		protected.GET("/storage/meta", app.storageHandler.Meta)
		protected.GET("/storage/status", app.storageHandler.Status)
		protected.POST("/storage/cleanup", app.storageHandler.Cleanup)

		// external MCP management
		protected.GET("/external-mcp", externalMCPHandler.GetExternalMCPs)
		protected.GET("/external-mcp/stats", externalMCPHandler.GetExternalMCPStats)
		protected.GET("/external-mcp/:name", externalMCPHandler.GetExternalMCP)
		protected.PUT("/external-mcp/:name", externalMCPHandler.AddOrUpdateExternalMCP)
		protected.DELETE("/external-mcp/:name", externalMCPHandler.DeleteExternalMCP)
		protected.POST("/external-mcp/:name/start", externalMCPHandler.StartExternalMCP)
		protected.POST("/external-mcp/:name/stop", externalMCPHandler.StopExternalMCP)

		// attack chain visualization
		protected.GET("/attack-chain/:conversationId", attackChainHandler.GetAttackChain)
		protected.POST("/attack-chain/:conversationId/regenerate", attackChainHandler.RegenerateAttackChain)

		// knowledge base management (always register routes; dynamically retrieve handler via App instance)
		knowledgeRoutes := protected.Group("/knowledge")
		{
			knowledgeRoutes.GET("/categories", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"categories": []string{},
						"enabled":    false,
						"message":    "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.GetCategories(c)
			})
			knowledgeRoutes.GET("/items", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"items":   []interface{}{},
						"enabled": false,
						"message": "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.GetItems(c)
			})
			knowledgeRoutes.GET("/items/:id", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"enabled": false,
						"message": "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.GetItem(c)
			})
			knowledgeRoutes.POST("/items", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"enabled": false,
						"error":   "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.CreateItem(c)
			})
			knowledgeRoutes.PUT("/items/:id", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"enabled": false,
						"error":   "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.UpdateItem(c)
			})
			knowledgeRoutes.DELETE("/items/:id", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"enabled": false,
						"error":   "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.DeleteItem(c)
			})
			knowledgeRoutes.GET("/index-status", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"enabled":          false,
						"total_items":      0,
						"indexed_items":    0,
						"progress_percent": 0,
						"is_complete":      false,
						"message":          "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.GetIndexStatus(c)
			})
			knowledgeRoutes.POST("/index", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"enabled": false,
						"error":   "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.StartIndex(c)
			})
			knowledgeRoutes.POST("/scan", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"enabled": false,
						"error":   "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.ScanKnowledgeBase(c)
			})
			knowledgeRoutes.GET("/retrieval-logs", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"logs":    []interface{}{},
						"enabled": false,
						"message": "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.GetRetrievalLogs(c)
			})
			knowledgeRoutes.DELETE("/retrieval-logs/:id", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"enabled": false,
						"error":   "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.DeleteRetrievalLog(c)
			})
			knowledgeRoutes.POST("/search", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"results": []interface{}{},
						"enabled": false,
						"message": "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.Search(c)
			})
			knowledgeRoutes.GET("/stats", func(c *gin.Context) {
				if app.knowledgeHandler == nil {
					c.JSON(http.StatusOK, gin.H{
						"enabled":          false,
						"total_categories": 0,
						"total_items":      0,
						"message":          "Knowledge base feature not enabled, please enable Knowledge retrieval in system settings",
					})
					return
				}
				app.knowledgeHandler.GetStats(c)
			})
		}

		// Vulnerability management
		protected.GET("/vulnerabilities", vulnerabilityHandler.ListVulnerabilities)
		protected.GET("/vulnerabilities/export", vulnerabilityHandler.ExportVulnerabilities)
		protected.DELETE("/vulnerabilities/batch", vulnerabilityHandler.BatchDeleteVulnerabilities)
		protected.GET("/vulnerabilities/filter-options", vulnerabilityHandler.GetVulnerabilityFilterOptions)
		protected.GET("/vulnerabilities/stats", vulnerabilityHandler.GetVulnerabilityStats)
		protected.GET("/vulnerability-alerts/subscription", vulnerabilityHandler.GetMyAlertSubscription)
		protected.PUT("/vulnerability-alerts/subscription", vulnerabilityHandler.UpdateMyAlertSubscription)
		protected.GET("/vulnerabilities/:id", vulnerabilityHandler.GetVulnerability)
		protected.POST("/vulnerabilities", vulnerabilityHandler.CreateVulnerability)
		protected.PUT("/vulnerabilities/:id", vulnerabilityHandler.UpdateVulnerability)
		protected.DELETE("/vulnerabilities/:id", vulnerabilityHandler.DeleteVulnerability)

		// project management and fact blackboard
		protected.GET("/projects/dashboard-summary", projectHandler.GetDashboardSummary)
		protected.GET("/projects", projectHandler.ListProjects)
		protected.POST("/projects", projectHandler.CreateProject)
		protected.GET("/projects/:id/stats", projectHandler.GetProjectStats)
		protected.GET("/projects/:id/conversations", projectHandler.ListProjectConversations)
		protected.GET("/projects/:id", projectHandler.GetProject)
		protected.PUT("/projects/:id", projectHandler.UpdateProject)
		protected.DELETE("/projects/:id", projectHandler.DeleteProject)
		protected.GET("/projects/:id/fact-graph", projectHandler.GetFactGraph)
		protected.GET("/projects/:id/fact-edges", projectHandler.ListFactEdges)
		protected.POST("/projects/:id/fact-edges", projectHandler.CreateFactEdge)
		protected.DELETE("/projects/:id/fact-edges/:edgeId", projectHandler.DeleteFactEdge)
		protected.POST("/projects/:id/promote-attack-chain/:conversationId", projectHandler.PromoteAttackChain)
		protected.GET("/projects/:id/facts", projectHandler.ListFacts)
		protected.POST("/projects/:id/facts", projectHandler.CreateFact)
		protected.PUT("/projects/:id/facts/:factId", projectHandler.UpdateFact)
		protected.DELETE("/projects/:id/facts/:factId", projectHandler.DeleteFact)
		protected.POST("/projects/:id/facts/deprecate", projectHandler.DeprecateFact)
		protected.POST("/projects/:id/facts/restore", projectHandler.RestoreFact)

		// WebShell management (proxy execution + connection config stored in SQLite)
		protected.GET("/webshell/connections", webshellHandler.ListConnections)
		protected.POST("/webshell/connections", webshellHandler.CreateConnection)
		protected.GET("/webshell/connections/:id/ai-history", webshellHandler.GetAIHistory)
		protected.GET("/webshell/connections/:id/ai-conversations", webshellHandler.ListAIConversations)
		protected.GET("/webshell/connections/:id/state", webshellHandler.GetConnectionState)
		protected.PUT("/webshell/connections/:id", webshellHandler.UpdateConnection)
		protected.PUT("/webshell/connections/:id/state", webshellHandler.SaveConnectionState)
		protected.DELETE("/webshell/connections/:id", webshellHandler.DeleteConnection)
		protected.POST("/webshell/exec", webshellHandler.Exec)
		protected.POST("/webshell/file", webshellHandler.FileOp)

		// C2 management (returns 503 when not enabled, to avoid nil handler pointer)
		c2Routes := protected.Group("/c2")
		c2Routes.Use(func(c *gin.Context) {
			if app.c2Manager == nil {
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
					"error":   "c2_disabled",
					"message": "C2 feature has been disabled in system settings",
					"enabled": false,
				})
				return
			}
			c.Next()
		})
		c2Routes.GET("/listeners", c2Handler.ListListeners)
		c2Routes.POST("/listeners", c2Handler.CreateListener)
		c2Routes.GET("/listeners/:id", c2Handler.GetListener)
		c2Routes.PUT("/listeners/:id", c2Handler.UpdateListener)
		c2Routes.DELETE("/listeners/:id", c2Handler.DeleteListener)
		c2Routes.POST("/listeners/:id/start", c2Handler.StartListener)
		c2Routes.POST("/listeners/:id/stop", c2Handler.StopListener)
		c2Routes.GET("/sessions", c2Handler.ListSessions)
		c2Routes.DELETE("/sessions", c2Handler.DeleteSessions)
		c2Routes.GET("/sessions/:id", c2Handler.GetSession)
		c2Routes.DELETE("/sessions/:id", c2Handler.DeleteSession)
		c2Routes.PUT("/sessions/:id/sleep", c2Handler.SetSessionSleep)
		c2Routes.PUT("/sessions/:id/note", c2Handler.SetSessionNote)
		c2Routes.GET("/tasks", c2Handler.ListTasks)
		c2Routes.DELETE("/tasks", c2Handler.DeleteTasks)
		c2Routes.GET("/tasks/:id", c2Handler.GetTask)
		c2Routes.POST("/tasks", c2Handler.CreateTask)
		c2Routes.POST("/tasks/:id/cancel", c2Handler.CancelTask)
		c2Routes.GET("/tasks/:id/wait", c2Handler.WaitTask)
		c2Routes.POST("/sessions/:id/tasks", c2Handler.CreateTask)
		c2Routes.POST("/payloads/oneliner", c2Handler.PayloadOneliner)
		c2Routes.POST("/payloads/build", c2Handler.PayloadBuild)
		c2Routes.GET("/payloads/:id/download", c2Handler.PayloadDownload)
		c2Routes.GET("/events", c2Handler.ListEvents)
		c2Routes.DELETE("/events", c2Handler.DeleteEvents)
		c2Routes.GET("/events/stream", c2Handler.EventStream)
		c2Routes.POST("/files/upload", c2Handler.UploadFileForImplant)
		c2Routes.GET("/files", c2Handler.ListFiles)
		c2Routes.GET("/tasks/:id/result-file", c2Handler.DownloadResultFile)
		c2Routes.GET("/profiles", c2Handler.ListProfiles)
		c2Routes.GET("/profiles/:id", c2Handler.GetProfile)
		c2Routes.POST("/profiles", c2Handler.CreateProfile)
		c2Routes.PUT("/profiles/:id", c2Handler.UpdateProfile)
		c2Routes.DELETE("/profiles/:id", c2Handler.DeleteProfile)

		// conversation attachment (chat_uploads) management
		protected.GET("/chat-uploads", chatUploadsHandler.List)
		protected.GET("/chat-uploads/export", chatUploadsHandler.Export)
		protected.GET("/chat-uploads/download", chatUploadsHandler.Download)
		protected.GET("/chat-uploads/path", chatUploadsHandler.ResolvePath)
		protected.GET("/chat-uploads/content", chatUploadsHandler.GetContent)
		protected.POST("/chat-uploads", chatUploadsHandler.Upload)
		protected.POST("/chat-uploads/mkdir", chatUploadsHandler.Mkdir)
		protected.DELETE("/chat-uploads", chatUploadsHandler.Delete)
		protected.PUT("/chat-uploads/rename", chatUploadsHandler.Rename)
		protected.PUT("/chat-uploads/content", chatUploadsHandler.PutContent)

		// role management
		protected.GET("/roles", roleHandler.GetRoles)
		protected.GET("/roles/:name", roleHandler.GetRole)
		protected.POST("/roles", roleHandler.CreateRole)
		protected.PUT("/roles/:name", roleHandler.UpdateRole)
		protected.DELETE("/roles/:name", roleHandler.DeleteRole)

		// workflow definition (graph structure is fixed; business fields saved in graph_json)
		protected.GET("/workflows/runs/pending", workflowHandler.ListPendingRuns)
		protected.GET("/workflows/runs/:runId/replay", workflowHandler.ReplayRun)
		protected.GET("/workflows/runs/:runId", workflowHandler.GetRun)
		protected.POST("/workflows/runs/:runId/resume", workflowHandler.ResumeRun)
		protected.POST("/workflows/validate", workflowHandler.Validate)
		protected.POST("/workflows/dry-run", workflowHandler.DryRun)
		protected.POST("/workflows/generate-draft", workflowHandler.GenerateDraft)
		protected.GET("/workflows/:id/package", workflowHandler.ExportPackage)
		protected.POST("/workflow-package-inspections", workflowHandler.CreatePackageInspection)
		protected.GET("/workflow-package-inspections/:inspectionId", workflowHandler.GetPackageInspection)
		protected.POST("/workflow-package-imports", workflowHandler.ApplyPackageImport)
		protected.GET("/workflow-package-imports/:importId", workflowHandler.GetPackageImport)
		protected.GET("/workflows", workflowHandler.List)
		protected.GET("/workflows/:id", workflowHandler.Get)
		protected.POST("/workflows", workflowHandler.Create)
		protected.PUT("/workflows/:id", workflowHandler.Update)
		protected.DELETE("/workflows/:id", workflowHandler.Delete)

		// Skills management (specific paths must be registered before /skills/:name)
		protected.GET("/skills", skillsHandler.GetSkills)
		protected.GET("/skills/stats", skillsHandler.GetSkillStats)
		protected.DELETE("/skills/stats", skillsHandler.ClearSkillStats)
		protected.GET("/skills/:name/files", skillsHandler.ListSkillPackageFiles)
		protected.GET("/skills/:name/file", skillsHandler.GetSkillPackageFile)
		protected.PUT("/skills/:name/file", skillsHandler.PutSkillPackageFile)
		protected.GET("/skills/:name/bound-roles", skillsHandler.GetSkillBoundRoles)
		protected.POST("/skills", skillsHandler.CreateSkill)
		protected.PUT("/skills/:name", skillsHandler.UpdateSkill)
		protected.DELETE("/skills/:name", skillsHandler.DeleteSkill)
		protected.DELETE("/skills/:name/stats", skillsHandler.ClearSkillStatsByName)
		protected.GET("/skills/:name", skillsHandler.GetSkill)

		// MCP endpoint
		protected.POST("/mcp", func(c *gin.Context) {
			mcpServer.HandleHTTP(c.Writer, c.Request)
		})

		// OpenAPI result aggregation endpoint (optional, for getting full conversation results)
		protected.GET("/conversations/:id/results", openAPIHandler.GetConversationResults)
	}

	// OpenAPI spec (requires auth to avoid exposing API structure info)
	protected.GET("/openapi/spec", openAPIHandler.GetOpenAPISpec)

	// API documentation page (publicly accessible, but API requires login)
	router.GET("/api-docs", func(c *gin.Context) {
		c.HTML(http.StatusOK, "api-docs.html", nil)
	})

	// static files
	router.Static("/static", "./web/static")
	router.LoadHTMLGlob("web/templates/*")

	// frontend pages
	router.GET("/", func(c *gin.Context) {
		version := app.config.Version
		if version == "" {
			version = "v1.0.0"
		}
		c.HTML(http.StatusOK, "index.html", gin.H{"Version": version})
	})
}

// registerWebshellTools registers WebShell-related MCP tools for AI assistant to execute commands and file operations on specified connections
func registerWebshellTools(mcpServer *mcp.Server, db *database.DB, webshellHandler *handler.WebShellHandler, logger *zap.Logger) {
	if db == nil || webshellHandler == nil {
		logger.Warn("skipping WebShell tool registration: db or webshellHandler is nil")
		return
	}

	// webshell_exec
	execTool := mcp.Tool{
		Name:             builtin.ToolWebshellExec,
		Description:      "Execute a system command on the specified WebShell connection and return stdout. connection_id is selected by the user in the AI assistant context.",
		ShortDescription: "Execute command on WebShell connection",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"connection_id": map[string]interface{}{
					"type":        "string",
					"description": "WebShell connection ID (e.g. ws_xxx)",
				},
				"command": map[string]interface{}{
					"type":        "string",
					"description": "System command to execute",
				},
			},
			"required": []string{"connection_id", "command"},
		},
	}
	execHandler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		cid, _ := args["connection_id"].(string)
		cmd, _ := args["command"].(string)
		if cid == "" || cmd == "" {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "connection_id and command are both required"}}, IsError: true}, nil
		}
		conn, err := db.GetWebshellConnection(cid)
		if err != nil || conn == nil {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "specified WebShell connection not found or query failed"}}, IsError: true}, nil
		}
		output, ok, errMsg := webshellHandler.ExecWithConnection(conn, cmd)
		if errMsg != "" {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: errMsg}}, IsError: true}, nil
		}
		if !ok {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "HTTP non-200, output:\n" + output}}, IsError: false}, nil
		}
		return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: output}}, IsError: false}, nil
	}
	mcpServer.RegisterTool(execTool, execHandler)

	// webshell_file_list
	listTool := mcp.Tool{
		Name:             builtin.ToolWebshellFileList,
		Description:      "List directory contents on the specified WebShell connection. Path defaults to current directory (.).",
		ShortDescription: "List directory on WebShell",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"connection_id": map[string]interface{}{"type": "string", "description": "WebShell connection ID"},
				"path":          map[string]interface{}{"type": "string", "description": "Directory path, defaults to ."},
			},
			"required": []string{"connection_id"},
		},
	}
	listHandler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		cid, _ := args["connection_id"].(string)
		path, _ := args["path"].(string)
		if cid == "" {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "connection_id is required"}}, IsError: true}, nil
		}
		conn, err := db.GetWebshellConnection(cid)
		if err != nil || conn == nil {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "WebShell connection not found"}}, IsError: true}, nil
		}
		output, ok, errMsg := webshellHandler.FileOpWithConnection(conn, "list", path, "", "")
		if errMsg != "" {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: errMsg}}, IsError: true}, nil
		}
		return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: output}}, IsError: !ok}, nil
	}
	mcpServer.RegisterTool(listTool, listHandler)

	// webshell_file_read
	readTool := mcp.Tool{
		Name:             builtin.ToolWebshellFileRead,
		Description:      "Read file content from the specified WebShell connection.",
		ShortDescription: "Read file on WebShell",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"connection_id": map[string]interface{}{"type": "string", "description": "WebShell connection ID"},
				"path":          map[string]interface{}{"type": "string", "description": "file path"},
			},
			"required": []string{"connection_id", "path"},
		},
	}
	readHandler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		cid, _ := args["connection_id"].(string)
		path, _ := args["path"].(string)
		if cid == "" || path == "" {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "connection_id and path are required"}}, IsError: true}, nil
		}
		conn, err := db.GetWebshellConnection(cid)
		if err != nil || conn == nil {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "WebShell connection not found"}}, IsError: true}, nil
		}
		output, ok, errMsg := webshellHandler.FileOpWithConnection(conn, "read", path, "", "")
		if errMsg != "" {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: errMsg}}, IsError: true}, nil
		}
		return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: output}}, IsError: !ok}, nil
	}
	mcpServer.RegisterTool(readTool, readHandler)

	// webshell_file_write
	writeTool := mcp.Tool{
		Name:             builtin.ToolWebshellFileWrite,
		Description:      "Write file content to the specified WebShell connection (overwrites existing file).",
		ShortDescription: "Write file on WebShell",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"connection_id": map[string]interface{}{"type": "string", "description": "WebShell connection ID"},
				"path":          map[string]interface{}{"type": "string", "description": "file path"},
				"content":       map[string]interface{}{"type": "string", "description": "Content to write"},
			},
			"required": []string{"connection_id", "path", "content"},
		},
	}
	writeHandler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		cid, _ := args["connection_id"].(string)
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		if cid == "" || path == "" {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "connection_id and path are required"}}, IsError: true}, nil
		}
		conn, err := db.GetWebshellConnection(cid)
		if err != nil || conn == nil {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "WebShell connection not found"}}, IsError: true}, nil
		}
		output, ok, errMsg := webshellHandler.FileOpWithConnection(conn, "write", path, content, "")
		if errMsg != "" {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: errMsg}}, IsError: true}, nil
		}
		if !ok {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "write may have failed, output:\n" + output}}, IsError: false}, nil
		}
		return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "write successful\n" + output}}, IsError: false}, nil
	}
	mcpServer.RegisterTool(writeTool, writeHandler)

	logger.Debug("WebShell tools registered successfully")
}

// registerWebshellManagementTools registers WebShell connection management MCP tools
func registerWebshellManagementTools(mcpServer *mcp.Server, db *database.DB, webshellHandler *handler.WebShellHandler, logger *zap.Logger) {
	if db == nil {
		logger.Warn("skipping WebShell management tool registration: db is nil")
		return
	}
	projectIDFromToolArgs := func(ctx context.Context, args map[string]interface{}) string {
		projectID, _ := args["project_id"].(string)
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			projectID = strings.TrimSpace(mcp.MCPProjectIDFromContext(ctx))
		}
		return projectID
	}
	explicitProjectIDFromToolArgs := func(args map[string]interface{}) string {
		projectID, _ := args["project_id"].(string)
		return strings.TrimSpace(projectID)
	}
	authorizeWebshellToolProject := func(principal authctx.Principal, permission, projectID string) *mcp.ToolResult {
		projectID = strings.TrimSpace(projectID)
		if projectID == "" {
			return nil
		}
		if projectID == database.ProjectFilterUnbound {
			return nil
		}
		if !db.UserCanAccessResource(principal.UserID, principal.ScopeFor(permission), "project", projectID) {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "access deniedproject: " + projectID}},
				IsError: true,
			}
		}
		return nil
	}

	// manage_webshell_list - list all webshell connections
	listTool := mcp.Tool{
		Name:             builtin.ToolManageWebshellList,
		Description:      "List saved WebShell connections, returning Connection ID, URL, type, project, remark and other info. Default: filtered by current conversation project boundary (project conversations see their own project; unbound conversations see unbound connections); pass project_id explicitly to filter by that project.",
		ShortDescription: "List all WebShell connections",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_id": map[string]interface{}{
					"type":        "string",
					"description": "Project ID; defaults to current project in project sessions if not specified.",
				},
			},
		},
	}
	listHandler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		connections := []database.WebShellConnection{}
		var err error
		if principal, ok := authctx.PrincipalFromContext(ctx); ok {
			projectID := explicitProjectIDFromToolArgs(args)
			if projectID == "" {
				projectID = mcpEffectiveProjectFilter(ctx, db)
			}
			if result := authorizeWebshellToolProject(principal, "webshell:read", projectID); result != nil {
				return result, nil
			}
			connections, err = db.ListWebshellConnectionsForAccess(principal.UserID, principal.ScopeFor("webshell:read"), projectID)
		} else {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "missing authentication identity"}}, IsError: true}, nil
		}
		if err != nil {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "failed to get connection list: " + err.Error()}},
				IsError: true,
			}, nil
		}
		if len(connections) == 0 {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "no WebShell connections found"}},
				IsError: false,
			}, nil
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Found %d WebShell connection(s):\n\n", len(connections)))
		for _, conn := range connections {
			sb.WriteString(fmt.Sprintf("ID: %s\n", conn.ID))
			sb.WriteString(fmt.Sprintf("  URL: %s\n", conn.URL))
			sb.WriteString(fmt.Sprintf("  type: %s\n", conn.Type))
			sb.WriteString(fmt.Sprintf("  Request method: %s\n", conn.Method))
			sb.WriteString(fmt.Sprintf("  command arguments: %s\n", conn.CmdParam))
			if conn.ProjectID != "" {
				sb.WriteString(fmt.Sprintf("  project ID: %s\n", conn.ProjectID))
			} else {
				sb.WriteString("  project: not bound\n")
			}
			if conn.Remark != "" {
				sb.WriteString(fmt.Sprintf("  Remark: %s\n", conn.Remark))
			}
			sb.WriteString(fmt.Sprintf("  created at: %s\n", conn.CreatedAt.Format("2006-01-02 15:04:05")))
			sb.WriteString("\n")
		}
		return &mcp.ToolResult{
			Content: []mcp.Content{{Type: "text", Text: sb.String()}},
			IsError: false,
		}, nil
	}
	mcpServer.RegisterTool(listTool, listHandler)

	// manage_webshell_add - add a new webshell connection
	addTool := mcp.Tool{
		Name:             builtin.ToolManageWebshellAdd,
		Description:      "Add a new WebShell connection to the management system. Supports PHP, ASP, ASPX, JSP and other types of web shells.",
		ShortDescription: "Add WebShell connection",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "Shell URL, e.g. http://target.com/shell.php (required)",
				},
				"password": map[string]interface{}{
					"type":        "string",
					"description": "Connection password/key, e.g. password for Behinder/AntSword",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Shell type: php, asp, aspx, jsp, defaults to php",
					"enum":        []string{"php", "asp", "aspx", "jsp"},
				},
				"method": map[string]interface{}{
					"type":        "string",
					"description": "Request method: GET or POST, defaults to POST",
					"enum":        []string{"GET", "POST"},
				},
				"cmd_param": map[string]interface{}{
					"type":        "string",
					"description": "Command argument name, defaults to cmd if not specified",
				},
				"remark": map[string]interface{}{
					"type":        "string",
					"description": "Remark, an identifiable remark name",
				},
			},
			"required": []string{"url"},
		},
	}
	addHandler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		urlStr, _ := args["url"].(string)
		if urlStr == "" {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "error: url is required"}},
				IsError: true,
			}, nil
		}

		password, _ := args["password"].(string)
		shellType, _ := args["type"].(string)
		if shellType == "" {
			shellType = "php"
		}
		method, _ := args["method"].(string)
		if method == "" {
			method = "post"
		}
		cmdParam, _ := args["cmd_param"].(string)
		if cmdParam == "" {
			cmdParam = "cmd"
		}
		remark, _ := args["remark"].(string)
		principal, ok := authctx.PrincipalFromContext(ctx)
		if !ok {
			return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "missing authentication identity"}}, IsError: true}, nil
		}
		projectID := projectIDFromToolArgs(ctx, args)
		if result := authorizeWebshellToolProject(principal, "webshell:write", projectID); result != nil {
			return result, nil
		}

		// generate connection ID
		connID := "ws_" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
		conn := &database.WebShellConnection{
			ID:        connID,
			URL:       urlStr,
			Password:  password,
			Type:      strings.ToLower(shellType),
			Method:    strings.ToLower(method),
			CmdParam:  cmdParam,
			Remark:    remark,
			ProjectID: projectID,
			CreatedAt: time.Now(),
		}

		if err := db.CreateWebshellConnection(conn); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "failed to add WebShell connection: " + err.Error()}},
				IsError: true,
			}, nil
		}
		_ = db.SetResourceOwner("webshell", conn.ID, principal.UserID)
		_ = db.AssignResourceToUser(principal.UserID, "webshell", conn.ID)
		projectLine := "project: not bound"
		if conn.ProjectID != "" {
			projectLine = "project ID: " + conn.ProjectID
		}

		return &mcp.ToolResult{
			Content: []mcp.Content{{
				Type: "text",
				Text: fmt.Sprintf("WebShell connection added successfully!\n\nConnection ID: %s\nURL: %s\nType: %s\nRequest method: %s\nCommand argument: %s\n%s", conn.ID, conn.URL, conn.Type, conn.Method, conn.CmdParam, projectLine),
			}},
			IsError: false,
		}, nil
	}
	mcpServer.RegisterTool(addTool, addHandler)

	// manage_webshell_update - update webshell connection
	updateTool := mcp.Tool{
		Name:             builtin.ToolManageWebshellUpdate,
		Description:      "Update info for an existing WebShell connection.",
		ShortDescription: "Update WebShell connection",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"connection_id": map[string]interface{}{
					"type":        "string",
					"description": "WebShell connection ID to update (required)",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "New shell URL",
				},
				"password": map[string]interface{}{
					"type":        "string",
					"description": "New connection password/key",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "New shell type: php, asp, aspx, jsp",
					"enum":        []string{"php", "asp", "aspx", "jsp"},
				},
				"method": map[string]interface{}{
					"type":        "string",
					"description": "New request method: GET or POST",
					"enum":        []string{"GET", "POST"},
				},
				"cmd_param": map[string]interface{}{
					"type":        "string",
					"description": "New command argument name",
				},
				"remark": map[string]interface{}{
					"type":        "string",
					"description": "New remark",
				},
				"project_id": map[string]interface{}{
					"type":        "string",
					"description": "New project ID; pass empty string to unbind.",
				},
			},
			"required": []string{"connection_id"},
		},
	}
	updateHandler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		connID, _ := args["connection_id"].(string)
		if connID == "" {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "error: connection_id is required"}},
				IsError: true,
			}, nil
		}

		// get existing connection
		existing, err := db.GetWebshellConnection(connID)
		if err != nil || existing == nil {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "specified WebShell connection not found: " + connID}},
				IsError: true,
			}, nil
		}

		// update fields (if new values are provided)
		if urlStr, ok := args["url"].(string); ok && urlStr != "" {
			existing.URL = urlStr
		}
		if password, ok := args["password"].(string); ok {
			existing.Password = password
		}
		if shellType, ok := args["type"].(string); ok && shellType != "" {
			existing.Type = strings.ToLower(shellType)
		}
		if method, ok := args["method"].(string); ok && method != "" {
			existing.Method = strings.ToLower(method)
		}
		if cmdParam, ok := args["cmd_param"].(string); ok && cmdParam != "" {
			existing.CmdParam = cmdParam
		}
		if remark, ok := args["remark"].(string); ok {
			existing.Remark = remark
		}
		if projectID, ok := args["project_id"].(string); ok {
			projectID = strings.TrimSpace(projectID)
			if projectID != "" {
				principal, ok := authctx.PrincipalFromContext(ctx)
				if !ok {
					return &mcp.ToolResult{Content: []mcp.Content{{Type: "text", Text: "missing authentication identity"}}, IsError: true}, nil
				}
				if result := authorizeWebshellToolProject(principal, "webshell:write", projectID); result != nil {
					return result, nil
				}
			}
			existing.ProjectID = projectID
		}

		if err := db.UpdateWebshellConnection(existing); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "update WebShell connection failed: " + err.Error()}},
				IsError: true,
			}, nil
		}

		return &mcp.ToolResult{
			Content: []mcp.Content{{
				Type: "text",
				Text: fmt.Sprintf("WebShell connection updated successfully!\n\nConnection ID: %s\nURL: %s\nType: %s\nRequest method: %s\nCommand argument: %s\nProject ID: %s\nRemark: %s", existing.ID, existing.URL, existing.Type, existing.Method, existing.CmdParam, existing.ProjectID, existing.Remark),
			}},
			IsError: false,
		}, nil
	}
	mcpServer.RegisterTool(updateTool, updateHandler)

	// manage_webshell_delete - delete webshell connection
	deleteTool := mcp.Tool{
		Name:             builtin.ToolManageWebshellDelete,
		Description:      "Delete the specified WebShell connection.",
		ShortDescription: "Delete WebShell connection",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"connection_id": map[string]interface{}{
					"type":        "string",
					"description": "WebShell connection ID to delete (required)",
				},
			},
			"required": []string{"connection_id"},
		},
	}
	deleteHandler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		connID, _ := args["connection_id"].(string)
		if connID == "" {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "error: connection_id is required"}},
				IsError: true,
			}, nil
		}

		if err := db.DeleteWebshellConnection(connID); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "delete WebShell connection failed: " + err.Error()}},
				IsError: true,
			}, nil
		}

		return &mcp.ToolResult{
			Content: []mcp.Content{{
				Type: "text",
				Text: fmt.Sprintf("WebShell connection %s deleted successfully", connID),
			}},
			IsError: false,
		}, nil
	}
	mcpServer.RegisterTool(deleteTool, deleteHandler)

	// manage_webshell_test - test webshell connection
	testTool := mcp.Tool{
		Name:             builtin.ToolManageWebshellTest,
		Description:      "Test whether the specified WebShell connection is available by attempting to execute a simple command (e.g. whoami or dir).",
		ShortDescription: "Test WebShell connection",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"connection_id": map[string]interface{}{
					"type":        "string",
					"description": "WebShell connection ID to test (required)",
				},
				"command": map[string]interface{}{
					"type":        "string",
					"description": "Test command, defaults to whoami (Linux) or dir (Windows)",
				},
			},
			"required": []string{"connection_id"},
		},
	}
	testHandler := func(ctx context.Context, args map[string]interface{}) (*mcp.ToolResult, error) {
		connID, _ := args["connection_id"].(string)
		if connID == "" {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "error: connection_id is required"}},
				IsError: true,
			}, nil
		}

		// get connection
		conn, err := db.GetWebshellConnection(connID)
		if err != nil || conn == nil {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: "specified WebShell connection not found: " + connID}},
				IsError: true,
			}, nil
		}

		// OK test command
		testCmd, _ := args["command"].(string)
		if testCmd == "" {
			// select default command based on shell type
			if conn.Type == "asp" || conn.Type == "aspx" {
				testCmd = "dir"
			} else {
				testCmd = "whoami"
			}
		}

		// execute test command
		output, ok, errMsg := webshellHandler.ExecWithConnection(conn, testCmd)
		if errMsg != "" {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: fmt.Sprintf("Connection test failed!\n\nConnection ID: %s\nURL: %s\nError: %s", connID, conn.URL, errMsg)}},
				IsError: true,
			}, nil
		}

		if !ok {
			return &mcp.ToolResult{
				Content: []mcp.Content{{Type: "text", Text: fmt.Sprintf("Connection test failed! HTTP non-200\n\nConnection ID: %s\nURL: %s\nOutput: %s", connID, conn.URL, output)}},
				IsError: true,
			}, nil
		}

		return &mcp.ToolResult{
			Content: []mcp.Content{{
				Type: "text",
				Text: fmt.Sprintf("Connection test successful!\n\nConnection ID: %s\nURL: %s\nType: %s\n\nTest command: %s\nOutput:\n%s", connID, conn.URL, conn.Type, testCmd, output),
			}},
			IsError: false,
		}, nil
	}
	mcpServer.RegisterTool(testTool, testHandler)

	logger.Debug("WebShell management tools registered successfully")
}

// initializeKnowledge initializes knowledge base components (for dynamic initialization)
func initializeKnowledge(
	cfg *config.Config,
	db *database.DB,
	knowledgeDBConn *database.DB,
	mcpServer *mcp.Server,
	agentHandler *handler.AgentHandler,
	app *App, // pass App reference to update knowledge base components
	logger *zap.Logger,
) (*handler.KnowledgeHandler, error) {
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
		knowledgeDBConn, err = database.NewKnowledgeDB(knowledgeDBPath, logger)
		if err != nil {
			return nil, fmt.Errorf("initializing knowledge base database failed: %w", err)
		}
		knowledgeDB = knowledgeDBConn.DB
		logger.Info("using separate knowledge base database", zap.String("path", knowledgeDBPath))
	} else {
		// backward compatible: use session database
		knowledgeDB = db.DB
		logger.Info("using session database for knowledge base data (recommended: configure knowledge_db_path to separate data)")
	}

	// create knowledge base manager
	knowledgeManager := knowledge.NewManager(knowledgeDB, cfg.Knowledge.BasePath, logger)

	// create embedder
	// use OpenAI config's API Key (if not specified in knowledge base configuration)
	if cfg.Knowledge.Embedding.APIKey == "" {
		cfg.Knowledge.Embedding.APIKey = cfg.OpenAI.APIKey
	}
	if cfg.Knowledge.Embedding.BaseURL == "" {
		cfg.Knowledge.Embedding.BaseURL = cfg.OpenAI.BaseURL
	}

	embedder, err := knowledge.NewEmbedder(context.Background(), &cfg.Knowledge, &cfg.OpenAI, logger)
	if err != nil {
		return nil, fmt.Errorf("initializing knowledge base embedder failed: %w", err)
	}

	// create retriever (Eino MultiQuery + reranking pipeline)
	retrievalConfig := knowledge.RetrievalConfigFromYAML(cfg.Knowledge.Retrieval)
	knowledgeRetriever := knowledge.NewRetriever(knowledgeDB, embedder, retrievalConfig, logger)
	if err := knowledge.WireRetrieverPipeline(context.Background(), knowledgeRetriever, &cfg.OpenAI); err != nil {
		return nil, fmt.Errorf("initializing knowledge base retrieval pipeline failed: %w", err)
	}

	// create indexer (Eino Compose chain)
	knowledgeIndexer, err := knowledge.NewIndexer(context.Background(), knowledgeDB, embedder, logger, &cfg.Knowledge)
	if err != nil {
		return nil, fmt.Errorf("initializing knowledge base indexer failed: %w", err)
	}

	// register knowledge retrieval tools to MCP server
	knowledge.RegisterKnowledgeTool(mcpServer, knowledgeRetriever, knowledgeManager, logger)

	// create knowledge base API handler
	knowledgeHandler := handler.NewKnowledgeHandler(knowledgeManager, knowledgeRetriever, knowledgeIndexer, db, logger)
	if app != nil && app.auditSvc != nil {
		knowledgeHandler.SetAudit(app.auditSvc)
	}
	logger.Info("knowledge base module initialization completed", zap.Bool("handler_created", knowledgeHandler != nil))

	// set knowledge base manager on AgentHandler to record retrieval logs
	agentHandler.SetKnowledgeManager(knowledgeManager)

	// update knowledge base components in App (if App is not nil, dynamic initialization has occurred)
	if app != nil {
		app.knowledgeManager = knowledgeManager
		app.knowledgeRetriever = knowledgeRetriever
		app.knowledgeIndexer = knowledgeIndexer
		app.knowledgeHandler = knowledgeHandler
		// if using separate database, update knowledgeDB
		if knowledgeDBPath != "" {
			app.knowledgeDB = knowledgeDBConn
		}
		logger.Info("knowledge base components in App updated")
	}

	// scan knowledge base and build index (async)
	go func() {
		itemsToIndex, err := knowledgeManager.ScanKnowledgeBase()
		if err != nil {
			logger.Warn("scanning knowledge base failed", zap.Error(err))
			return
		}

		// check whether index already exists
		hasIndex, err := knowledgeIndexer.HasIndex()
		if err != nil {
			logger.Warn("checkindexstatusfailed", zap.Error(err))
			return
		}

		if hasIndex {
			// if index already exists, only index newly added or updated items
			if len(itemsToIndex) > 0 {
				logger.Info("existing knowledge base index detected, starting incremental indexing", zap.Int("count", len(itemsToIndex)))
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
							logger.Warn("indexing knowledge item failed", zap.String("itemId", itemID), zap.Error(err))
						}

						// if 2 consecutive failures, immediately stop incremental indexing
						if consecutiveFailures >= 2 {
							logger.Error("too many consecutive index failures, stopping incremental indexing immediately",
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
				logger.Info("incremental indexing completed", zap.Int("totalItems", len(itemsToIndex)), zap.Int("failedCount", failedCount))
			} else {
				logger.Info("existing knowledge base index detected, no new or updated items to index")
			}
			return
		}

		// cold start: only build index for knowledge items without vectors (consistent with IndexMissing semantics)
		logger.Info("no knowledge base index detected, starting automatic index build")
		ctx := context.Background()
		if err := knowledgeIndexer.IndexMissing(ctx); err != nil {
			logger.Warn("auto-build knowledge base indexing failed", zap.Error(err))
		}
	}()

	return knowledgeHandler, nil
}

// corsMiddleware allows same-origin requests, valid Chromium extension
// origins, and exact origins explicitly configured by the operator. CORS is
// not an authentication boundary; API access still requires a valid session.
func corsMiddleware(configuredOrigins []string) gin.HandlerFunc {
	allowedOrigins := make(map[string]struct{}, len(configuredOrigins))
	for _, origin := range configuredOrigins {
		if normalized, ok := normalizeCORSOrigin(origin); ok {
			allowedOrigins[normalized] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin != "" {
			c.Writer.Header().Add("Vary", "Origin")
			normalized, valid := normalizeCORSOrigin(origin)
			_, explicitlyAllowed := allowedOrigins[normalized]
			parsed, _ := url.Parse(origin)
			sameHost := valid && strings.EqualFold(parsed.Host, c.Request.Host)
			browserExtension := valid && isChromiumExtensionOrigin(parsed)
			if !sameHost && !browserExtension && !explicitlyAllowed {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "cross-origin request denied"})
				return
			}
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Max-Age", "600")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

// isChromiumExtensionOrigin accepts only Chrome's canonical 32-character
// extension IDs (letters a-p). It does not allow arbitrary custom schemes or
// web origins, and the extension must separately obtain host permission.
func isChromiumExtensionOrigin(origin *url.URL) bool {
	if origin == nil || !strings.EqualFold(origin.Scheme, "chrome-extension") || origin.Port() != "" {
		return false
	}
	id := strings.ToLower(origin.Hostname())
	if len(id) != 32 {
		return false
	}
	for _, ch := range id {
		if ch < 'a' || ch > 'p' {
			return false
		}
	}
	return true
}

// normalizeCORSOrigin validates and canonicalizes a serialized origin. CORS
// origins never contain credentials, paths, query strings, or fragments.
func normalizeCORSOrigin(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "*" || strings.EqualFold(raw, "null") {
		return "", false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), true
}
