package handler

import (
	"context"
	"net/http"
	"sync"
	"time"

	"kestrel/internal/attackchain"
	"kestrel/internal/config"
	"kestrel/internal/database"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AttackChainHandler handles attack chain requests.
type AttackChainHandler struct {
	db           *database.DB
	logger       *zap.Logger
	openAIConfig *config.OpenAIConfig
	mu           sync.RWMutex // Protects concurrent access to openAIConfig.
	// Prevents concurrent generation for the same conversation.
	generatingLocks sync.Map // map[string]*sync.Mutex
}

// NewAttackChainHandler creates a new attack chain handler.
func NewAttackChainHandler(db *database.DB, openAIConfig *config.OpenAIConfig, logger *zap.Logger) *AttackChainHandler {
	return &AttackChainHandler{
		db:           db,
		logger:       logger,
		openAIConfig: openAIConfig,
	}
}

// UpdateConfig updates the OpenAI config.
func (h *AttackChainHandler) UpdateConfig(cfg *config.OpenAIConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.openAIConfig = cfg
	h.logger.Info("AttackChainHandler config updated",
		zap.String("base_url", cfg.BaseURL),
		zap.String("model", cfg.Model),
	)
}

// getOpenAIConfig returns the OpenAI config in a thread-safe manner.
func (h *AttackChainHandler) getOpenAIConfig() *config.OpenAIConfig {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.openAIConfig
}

// GetAttackChain returns the attack chain (generating on demand).
// GET /api/attack-chain/:conversationId
func (h *AttackChainHandler) GetAttackChain(c *gin.Context) {
	conversationID := c.Param("conversationId")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversationId is required"})
		return
	}

	// Check that the conversation exists.
	_, err := h.db.GetConversation(conversationID)
	if err != nil {
		h.logger.Warn("conversation not found", zap.String("conversationId", conversationID), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}

	// First try loading from the database (if already generated).
	openAIConfig := h.getOpenAIConfig()
	builder := attackchain.NewBuilder(h.db, openAIConfig, h.logger)
	chain, err := builder.LoadChainFromDatabase(conversationID)
	if err == nil && len(chain.Nodes) > 0 {
		// Already exists — return it directly.
		h.logger.Info("returning existing attack chain", zap.String("conversationId", conversationID))
		c.JSON(http.StatusOK, chain)
		return
	}

	// Not found — generate a new attack chain on demand.
	// Use a lock to prevent concurrent generation for the same conversation.
	lockInterface, _ := h.generatingLocks.LoadOrStore(conversationID, &sync.Mutex{})
	lock := lockInterface.(*sync.Mutex)

	// Try to acquire the lock; if already generating, return an error.
	acquired := lock.TryLock()
	if !acquired {
		h.logger.Info("attack chain is currently being generated, please try again later", zap.String("conversationId", conversationID))
		c.JSON(http.StatusConflict, gin.H{"error": "attack chain is currently being generated, please try again later"})
		return
	}
	defer lock.Unlock()

	// Check again whether it was generated while waiting for the lock.
	chain, err = builder.LoadChainFromDatabase(conversationID)
	if err == nil && len(chain.Nodes) > 0 {
		h.logger.Info("returning attack chain generated while waiting for lock", zap.String("conversationId", conversationID))
		c.JSON(http.StatusOK, chain)
		return
	}

	h.logger.Info("starting attack chain generation", zap.String("conversationId", conversationID))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	chain, err = builder.BuildChainFromConversation(ctx, conversationID)
	if err != nil {
		h.logger.Error("failed to generate attack chain", zap.String("conversationId", conversationID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate attack chain: " + err.Error()})
		return
	}

	// After generation, optionally delete from the lock map (keeping it also prevents rapid re-generation).
	// h.generatingLocks.Delete(conversationID)

	c.JSON(http.StatusOK, chain)
}

// RegenerateAttackChain regenerates the attack chain.
// POST /api/attack-chain/:conversationId/regenerate
func (h *AttackChainHandler) RegenerateAttackChain(c *gin.Context) {
	conversationID := c.Param("conversationId")
	if conversationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversationId is required"})
		return
	}

	// Check that the conversation exists.
	_, err := h.db.GetConversation(conversationID)
	if err != nil {
		h.logger.Warn("conversation not found", zap.String("conversationId", conversationID), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}

	// Delete the old attack chain.
	if err := h.db.DeleteAttackChain(conversationID); err != nil {
		h.logger.Warn("failed to delete old attack chain", zap.Error(err))
	}

	// Use a lock to prevent concurrent generation.
	lockInterface, _ := h.generatingLocks.LoadOrStore(conversationID, &sync.Mutex{})
	lock := lockInterface.(*sync.Mutex)

	acquired := lock.TryLock()
	if !acquired {
		h.logger.Info("attack chain is currently being generated, please try again later", zap.String("conversationId", conversationID))
		c.JSON(http.StatusConflict, gin.H{"error": "attack chain is currently being generated, please try again later"})
		return
	}
	defer lock.Unlock()

	// Generate new attack chain.
	h.logger.Info("regenerate attack chain", zap.String("conversationId", conversationID))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	openAIConfig := h.getOpenAIConfig()
	builder := attackchain.NewBuilder(h.db, openAIConfig, h.logger)
	chain, err := builder.BuildChainFromConversation(ctx, conversationID)
	if err != nil {
		h.logger.Error("failed to generate attack chain", zap.String("conversationId", conversationID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate attack chain: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, chain)
}
