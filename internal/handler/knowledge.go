package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"kestrel/internal/audit"
	"kestrel/internal/database"
	"kestrel/internal/knowledge"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// KnowledgeHandler is the knowledge base handler
type KnowledgeHandler struct {
	manager   *knowledge.Manager
	retriever *knowledge.Retriever
	indexer   *knowledge.Indexer
	db        *database.DB
	logger    *zap.Logger
	audit     *audit.Service
}

// SetAudit wires platform audit logging.
func (h *KnowledgeHandler) SetAudit(s *audit.Service) {
	h.audit = s
}

// NewKnowledgeHandler creates a new knowledge base handler
func NewKnowledgeHandler(
	manager *knowledge.Manager,
	retriever *knowledge.Retriever,
	indexer *knowledge.Indexer,
	db *database.DB,
	logger *zap.Logger,
) *KnowledgeHandler {
	return &KnowledgeHandler{
		manager:   manager,
		retriever: retriever,
		indexer:   indexer,
		db:        db,
		logger:    logger,
	}
}

// GetCategories returns all categories
func (h *KnowledgeHandler) GetCategories(c *gin.Context) {
	categories, err := h.manager.GetCategories()
	if err != nil {
		h.logger.Error("get categories failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"categories": categories})
}

// GetItems returns the knowledge item list (supports pagination by category and keyword search; full content not returned by default)
func (h *KnowledgeHandler) GetItems(c *gin.Context) {
	category := c.Query("category")
	searchKeyword := c.Query("search") // search keyword

	// if a search keyword is provided, execute keyword search (search all data)
	if searchKeyword != "" {
		items, err := h.manager.SearchItemsByKeyword(searchKeyword, category)
		if err != nil {
			h.logger.Error("search knowledge items failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// group results by category
		groupedByCategory := make(map[string][]*knowledge.KnowledgeItemSummary)
		for _, item := range items {
			cat := item.Category
			if cat == "" {
				cat = "Uncategorized"
			}
			groupedByCategory[cat] = append(groupedByCategory[cat], item)
		}

		// convert to CategoryWithItems format
		categoriesWithItems := make([]*knowledge.CategoryWithItems, 0, len(groupedByCategory))
		for cat, catItems := range groupedByCategory {
			categoriesWithItems = append(categoriesWithItems, &knowledge.CategoryWithItems{
				Category:  cat,
				ItemCount: len(catItems),
				Items:     catItems,
			})
		}

		// sort by category name
		for i := 0; i < len(categoriesWithItems)-1; i++ {
			for j := i + 1; j < len(categoriesWithItems); j++ {
				if categoriesWithItems[i].Category > categoriesWithItems[j].Category {
					categoriesWithItems[i], categoriesWithItems[j] = categoriesWithItems[j], categoriesWithItems[i]
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"categories": categoriesWithItems,
			"total":      len(categoriesWithItems),
			"search":     searchKeyword,
			"is_search":  true,
		})
		return
	}

	// pagination mode: categoryPage=true means paginate by category; otherwise paginate by item (backward compatible)
	categoryPageMode := c.Query("categoryPage") != "false" // default to category pagination

	// pagination parameters
	limit := 50 // default 50 per page (categories when paginating by category, items when paginating by item)
	offset := 0
	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := parseInt(limitStr); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if parsed, err := parseInt(offsetStr); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// if category parameter is specified and using category pagination mode, return only that category
	if category != "" && categoryPageMode {
		// single-category mode: return all knowledge items for this category (no pagination)
		items, total, err := h.manager.GetItemsSummary(category, 0, 0)
		if err != nil {
			h.logger.Error("get knowledge items failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// wrap in category structure
		categoriesWithItems := []*knowledge.CategoryWithItems{
			{
				Category:  category,
				ItemCount: total,
				Items:     items,
			},
		}

		c.JSON(http.StatusOK, gin.H{
			"categories": categoriesWithItems,
			"total":      1, // only one category
			"limit":      limit,
			"offset":     offset,
		})
		return
	}

	if categoryPageMode {
		// paginate by category mode (default)
		// limit is the number of categories per page, recommended 5-10
		if limit <= 0 || limit > 100 {
			limit = 10 // default 10 categories per page
		}

		categoriesWithItems, totalCategories, err := h.manager.GetCategoriesWithItems(limit, offset)
		if err != nil {
			h.logger.Error("get category knowledge items failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"categories": categoriesWithItems,
			"total":      totalCategories,
			"limit":      limit,
			"offset":     offset,
		})
		return
	}

	// paginate by item mode (backward compatible)
	// whether to include full content (default false, returns summary only)
	includeContent := c.Query("includeContent") == "true"

	if includeContent {
		// return full content (backward compatible)
		items, err := h.manager.GetItemsWithOptions(category, limit, offset, true)
		if err != nil {
			h.logger.Error("get knowledge items failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Get total count.
		total, err := h.manager.GetItemsCount(category)
		if err != nil {
			h.logger.Warn("get knowledge item count failed", zap.Error(err))
			total = len(items)
		}

		c.JSON(http.StatusOK, gin.H{
			"items":  items,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	} else {
		// return summary (no full content, recommended)
		items, total, err := h.manager.GetItemsSummary(category, limit, offset)
		if err != nil {
			h.logger.Error("get knowledge items failed", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"items":  items,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
	}
}

// GetItem returns a single knowledge item
func (h *KnowledgeHandler) GetItem(c *gin.Context) {
	id := c.Param("id")

	item, err := h.manager.GetItem(id)
	if err != nil {
		h.logger.Error("get knowledge items failed", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, item)
}

// CreateItem creates a knowledge item
func (h *KnowledgeHandler) CreateItem(c *gin.Context) {
	var req struct {
		Category string `json:"category" binding:"required"`
		Title    string `json:"title" binding:"required"`
		Content  string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.manager.CreateItem(req.Category, req.Title, req.Content)
	if err != nil {
		h.logger.Error("create knowledge item failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// async indexing
	go func() {
		ctx := context.Background()
		if err := h.indexer.IndexItem(ctx, item.ID); err != nil {
			h.logger.Warn("indexing knowledge item failed", zap.String("itemId", item.ID), zap.Error(err))
		}
	}()

	c.JSON(http.StatusOK, item)
}

// UpdateItem updates a knowledge item
func (h *KnowledgeHandler) UpdateItem(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		Category string `json:"category" binding:"required"`
		Title    string `json:"title" binding:"required"`
		Content  string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item, err := h.manager.UpdateItem(id, req.Category, req.Title, req.Content)
	if err != nil {
		h.logger.Error("update knowledge item failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// async re-indexing
	go func() {
		ctx := context.Background()
		if err := h.indexer.IndexItem(ctx, item.ID); err != nil {
			h.logger.Warn("re-index knowledge item failed", zap.String("itemId", item.ID), zap.Error(err))
		}
	}()

	c.JSON(http.StatusOK, item)
}

// DeleteItem deletes a knowledge item
func (h *KnowledgeHandler) DeleteItem(c *gin.Context) {
	id := c.Param("id")

	if err := h.manager.DeleteItem(id); err != nil {
		h.logger.Error("delete knowledge item failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if h.audit != nil {
		h.audit.RecordOK(c, "knowledge", "item_delete", "delete knowledge item", "knowledge_item", id, nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": "deletion successful"})
}

// StartIndex builds the knowledge base vector index. By default only indexes items missing vectors; mode=full performs a full rebuild.
func (h *KnowledgeHandler) StartIndex(c *gin.Context) {
	if err := h.indexer.TryBeginIndexRun(); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "An index task is already in progress, please wait for it to complete"})
		return
	}

	mode := strings.TrimSpace(c.Query("mode"))
	if mode == "" {
		mode = "missing"
	}
	if mode != "full" && mode != "missing" {
		h.indexer.FinishIndexRun()
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode parameter, valid values: missing, full"})
		return
	}

	fullRebuild := mode == "full"
	message := "Index build started and will run in the background"
	auditAction := "index_build"
	auditDetail := "Build knowledge base index"
	if fullRebuild {
		message = "Full index rebuild has started and will run in the background"
		auditAction = "index_rebuild_full"
		auditDetail = "Full rebuild of knowledge base index"
	}

	go func() {
		defer h.indexer.FinishIndexRun()
		ctx := context.Background()
		var err error
		if fullRebuild {
			err = h.indexer.RunRebuildIndex(ctx)
		} else {
			err = h.indexer.RunIndexMissing(ctx)
		}
		if err != nil {
			if fullRebuild {
				h.logger.Error("full index rebuild failed", zap.Error(err))
			} else {
				h.logger.Error("failed to build knowledge base index", zap.Error(err))
			}
		}
	}()

	if h.audit != nil {
		h.audit.RecordOK(c, "knowledge", auditAction, auditDetail, "knowledge", "", nil)
	}
	c.JSON(http.StatusOK, gin.H{"message": message, "mode": mode})
}

// ScanKnowledgeBase scans the knowledge base.
func (h *KnowledgeHandler) ScanKnowledgeBase(c *gin.Context) {
	itemsToIndex, err := h.manager.ScanKnowledgeBase()
	if err != nil {
		h.logger.Error("failed to scan knowledge base", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(itemsToIndex) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "scan complete, no new or updated items to index"})
		return
	}

	// Asynchronously index newly added or updated items (incremental index).
	go func() {
		ctx := context.Background()
		h.logger.Info("starting incremental index", zap.Int("count", len(itemsToIndex)))
		failedCount := 0
		consecutiveFailures := 0
		var firstFailureItemID string
		var firstFailureError error

		for i, itemID := range itemsToIndex {
			if err := h.indexer.IndexItem(ctx, itemID); err != nil {
				failedCount++
				consecutiveFailures++

				// Only log detailed info on the first failure.
				if consecutiveFailures == 1 {
					firstFailureItemID = itemID
					firstFailureError = err
					h.logger.Warn("indexing knowledge item failed",
						zap.String("itemId", itemID),
						zap.Int("totalItems", len(itemsToIndex)),
						zap.Error(err),
					)
				}

				// If 2 consecutive failures occur, immediately stop the incremental index.
				if consecutiveFailures >= 2 {
					h.logger.Error("too many consecutive index failures, stopping incremental indexing immediately",
						zap.Int("consecutiveFailures", consecutiveFailures),
						zap.Int("totalItems", len(itemsToIndex)),
						zap.Int("processedItems", i+1),
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

			// Reduce progress log frequency.
			if (i+1)%10 == 0 || i+1 == len(itemsToIndex) {
				h.logger.Info("index progress", zap.Int("current", i+1), zap.Int("total", len(itemsToIndex)), zap.Int("failed", failedCount))
			}
		}
		h.logger.Info("incremental indexing completed", zap.Int("totalItems", len(itemsToIndex)), zap.Int("failedCount", failedCount))
	}()

	c.JSON(http.StatusOK, gin.H{
		"message":        fmt.Sprintf("scan complete, starting indexing of %d new or updated knowledge items", len(itemsToIndex)),
		"items_to_index": len(itemsToIndex),
	})
}

// GetRetrievalLogs returns retrieval logs.
func (h *KnowledgeHandler) GetRetrievalLogs(c *gin.Context) {
	conversationID := c.Query("conversationId")
	messageID := c.Query("messageId")
	limit := 50 // Default 50 items.

	if limitStr := c.Query("limit"); limitStr != "" {
		if parsed, err := parseInt(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	logs, err := h.manager.GetRetrievalLogs(conversationID, messageID, limit)
	if err != nil {
		h.logger.Error("failed to get retrieval logs", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// DeleteRetrievalLog deletes a retrieval log.
func (h *KnowledgeHandler) DeleteRetrievalLog(c *gin.Context) {
	id := c.Param("id")

	if err := h.manager.DeleteRetrievalLog(id); err != nil {
		h.logger.Error("failed to delete retrieval log", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deletion successful"})
}

// GetIndexStatus returns the index status.
func (h *KnowledgeHandler) GetIndexStatus(c *gin.Context) {
	status, err := h.manager.GetIndexStatus()
	if err != nil {
		h.logger.Error("failed to get index status", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get the indexer's error info.
	if h.indexer != nil {
		lastError, lastErrorTime := h.indexer.GetLastError()
		// Keep a failed operation visible until the indexer clears it for the next run.
		appendKnowledgeIndexError(status, lastError, lastErrorTime)

		// Get the rebuild index status.
		isRebuilding, totalItems, current, failed, lastItemID, lastChunks, startTime := h.indexer.GetRebuildStatus()
		if isRebuilding {
			status["is_rebuilding"] = true
			status["rebuild_total"] = totalItems
			status["rebuild_current"] = current
			status["rebuild_failed"] = failed
			status["rebuild_start_time"] = startTime.Format(time.RFC3339)
			if lastItemID != "" {
				status["rebuild_last_item_id"] = lastItemID
			}
			if lastChunks > 0 {
				status["rebuild_last_chunks"] = lastChunks
			}
			// While rebuilding, is_complete is false.
			status["is_complete"] = false
			// Calculate rebuild progress percentage.
			if totalItems > 0 {
				status["progress_percent"] = float64(current) / float64(totalItems) * 100
			}
		}
	}

	c.JSON(http.StatusOK, status)
}

// Search searches the knowledge base (used for API calls; Agent internally uses Retriever).
func (h *KnowledgeHandler) Search(c *gin.Context) {
	var req knowledge.SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Retriever.Search goes through Eino VectorEinoRetriever, consistent with the MCP tool chain.
	results, err := h.retriever.Search(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("search knowledge base failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

// GetStats returns knowledge base statistics
func (h *KnowledgeHandler) GetStats(c *gin.Context) {
	totalCategories, totalItems, err := h.manager.GetStats()
	if err != nil {
		h.logger.Error("failed to get knowledge base statistics", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enabled":          true,
		"total_categories": totalCategories,
		"total_items":      totalItems,
	})
}

// Helper function: parse integer.
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}

func appendKnowledgeIndexError(status map[string]interface{}, message string, occurredAt time.Time) {
	if message == "" {
		return
	}
	status["last_error"] = message
	status["last_error_time"] = occurredAt.Format(time.RFC3339)
}
