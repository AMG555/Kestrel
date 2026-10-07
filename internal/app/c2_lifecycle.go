package app

import (
	"context"

	"kestrel/internal/c2"
	"kestrel/internal/config"
	"kestrel/internal/database"
	"kestrel/internal/handler"

	"go.uber.org/zap"
)

// setupC2Runtime creates the C2 Manager, watchdog, and cancellation function; does not register MCP tools (registered uniformly by Apply after ClearTools).
func setupC2Runtime(
	cfg *config.Config,
	db *database.DB,
	agentHandler *handler.AgentHandler,
	logger *zap.Logger,
) (*c2.Manager, *c2.SessionWatchdog, context.CancelFunc) {
	if !cfg.C2.EnabledEffective() {
		return nil, nil, nil
	}
	c2Manager := c2.NewManager(db, logger, "tmp/c2")
	c2Manager.Registry().Register(string(c2.ListenerTypeTCPReverse), c2.NewTCPReverseListener)
	c2Manager.Registry().Register(string(c2.ListenerTypeHTTPBeacon), c2.NewHTTPBeaconListener)
	c2Manager.Registry().Register(string(c2.ListenerTypeHTTPSBeacon), c2.NewHTTPSBeaconListener)
	c2Manager.Registry().Register(string(c2.ListenerTypeWebSocket), c2.NewWebSocketListener)
	c2HITLBridge := NewC2HITLBridge(db, logger)
	c2Manager.SetHITLBridge(c2HITLBridge)
	c2Manager.SetHITLDangerousGate(func(conversationID, toolName string) bool {
		return agentHandler.HITLNeedsToolApproval(conversationID, toolName)
	})
	c2Hooks := SetupC2Hooks(&C2HooksConfig{
		DB:     db,
		Logger: logger,
		AttackChainRecord: func(session *database.C2Session, phase string, description string) {
			logger.Info("C2 Attack Chain",
				zap.String("session_id", session.ID),
				zap.String("phase", phase),
				zap.String("desc", description),
			)
		},
		VulnRecord: func(session *database.C2Session, title string, severity string) {
			logger.Info("C2 Vulnerability",
				zap.String("session_id", session.ID),
				zap.String("title", title),
				zap.String("severity", severity),
			)
		},
	})
	c2Manager.SetHooks(c2Hooks)
	c2Manager.RestoreRunningListeners()
	c2Watchdog := c2.NewSessionWatchdog(c2Manager)
	watchdogCtx, watchdogCancel := context.WithCancel(context.Background())
	go c2Watchdog.Run(watchdogCtx)
	return c2Manager, c2Watchdog, watchdogCancel
}

// ReconcileC2AfterConfigApply starts or stops C2 based on the current in-memory config (does not write to disk; called in Apply before ClearTools).
func (a *App) ReconcileC2AfterConfigApply() error {
	if !a.config.C2.EnabledEffective() {
		a.shutdownC2()
		return nil
	}
	if a.c2Manager != nil {
		return nil
	}
	if a.db == nil || a.agentHandler == nil {
		return nil
	}
	m, wd, cancel := setupC2Runtime(a.config, a.db, a.agentHandler, a.logger.Logger)
	if m == nil {
		return nil
	}
	a.c2Manager = m
	a.c2Watchdog = wd
	a.c2WatchdogCancel = cancel
	if a.c2Handler != nil {
		a.c2Handler.SetManager(m)
	}
	a.logger.Info("C2 subsystem started per config")
	return nil
}

// shutdownC2 stops the watchdog and all listeners, and disconnects the Handler reference.
func (a *App) shutdownC2() {
	had := a.c2WatchdogCancel != nil || a.c2Manager != nil
	if a.c2WatchdogCancel != nil {
		a.c2WatchdogCancel()
		a.c2WatchdogCancel = nil
	}
	a.c2Watchdog = nil
	if a.c2Manager != nil {
		a.c2Manager.Close()
		a.c2Manager = nil
	}
	if a.c2Handler != nil {
		a.c2Handler.SetManager(nil)
	}
	if had {
		a.logger.Info("C2 subsystem closed")
	}
}
