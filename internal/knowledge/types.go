package knowledge

import (
	"encoding/json"
	"time"
)

// formatTime formats a time value as RFC3339; returns an empty string for zero time.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// KnowledgeItem is a knowledge base entry.
type KnowledgeItem struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"` // risk type (directory name)
	Title     string    `json:"title"`    // title (filename)
	FilePath  string    `json:"filePath"` // file path
	Content   string    `json:"content"`  // File content
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// KnowledgeItemSummary is a knowledge base entry summary (used for listings; excludes full content).
type KnowledgeItemSummary struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"`
	Title     string    `json:"title"`
	FilePath  string    `json:"filePath"`
	Content   string    `json:"content,omitempty"` // optional: content preview (if provided, usually only the first 150 characters)
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// MarshalJSON customizes JSON serialization to ensure times are formatted correctly.
func (k *KnowledgeItemSummary) MarshalJSON() ([]byte, error) {
	type Alias KnowledgeItemSummary
	aux := &struct {
		*Alias
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	}{
		Alias: (*Alias)(k),
	}
	aux.CreatedAt = formatTime(k.CreatedAt)
	aux.UpdatedAt = formatTime(k.UpdatedAt)
	return json.Marshal(aux)
}

// MarshalJSON customizes JSON serialization to ensure times are formatted correctly.
func (k *KnowledgeItem) MarshalJSON() ([]byte, error) {
	type Alias KnowledgeItem
	aux := &struct {
		*Alias
		CreatedAt string `json:"createdAt"`
		UpdatedAt string `json:"updatedAt"`
	}{
		Alias: (*Alias)(k),
	}
	aux.CreatedAt = formatTime(k.CreatedAt)
	aux.UpdatedAt = formatTime(k.UpdatedAt)
	return json.Marshal(aux)
}

// KnowledgeChunk is a knowledge fragment used for vectorization.
type KnowledgeChunk struct {
	ID         string    `json:"id"`
	ItemID     string    `json:"itemId"`
	ChunkIndex int       `json:"chunkIndex"`
	ChunkText  string    `json:"chunkText"`
	Embedding  []float32 `json:"-"` // vector embedding, not serialized to JSON
	CreatedAt  time.Time `json:"createdAt"`
}

// RetrievalResult retrieval results
type RetrievalResult struct {
	Chunk      *KnowledgeChunk `json:"chunk"`
	Item       *KnowledgeItem  `json:"item"`
	Similarity float64         `json:"similarity"` // similarity score
	Score      float64         `json:"score"`      // same as Similarity: cosine similarity
}

// RetrievalLog is a retrieval log entry.
type RetrievalLog struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId,omitempty"`
	MessageID      string    `json:"messageId,omitempty"`
	Query          string    `json:"query"`
	RiskType       string    `json:"riskType,omitempty"`
	RetrievedItems []string  `json:"retrievedItems"` // list of retrieved knowledge item IDs
	CreatedAt      time.Time `json:"createdAt"`
}

// MarshalJSON customizes JSON serialization to ensure times are formatted correctly.
func (r *RetrievalLog) MarshalJSON() ([]byte, error) {
	type Alias RetrievalLog
	return json.Marshal(&struct {
		*Alias
		CreatedAt string `json:"createdAt"`
	}{
		Alias:     (*Alias)(r),
		CreatedAt: formatTime(r.CreatedAt),
	})
}

// CategoryWithItems is a category and its knowledge items (used for pagination by category).
type CategoryWithItems struct {
	Category  string                  `json:"category"`  // category name
	ItemCount int                     `json:"itemCount"` // total knowledge items in this category
	Items     []*KnowledgeItemSummary `json:"items"`     // list of knowledge items in this category
}

// SearchRequest is a knowledge base search request.
type SearchRequest struct {
	Query          string  `json:"query"`
	RiskType       string  `json:"riskType,omitempty"`       // optional: specify risk type
	SubIndexFilter string  `json:"subIndexFilter,omitempty"` // optional: only keep rows whose sub_indexes contain this tag (includes untagged legacy data)
	TopK           int     `json:"topK,omitempty"`           // return top-K results, default 5
	Threshold      float64 `json:"threshold,omitempty"`      // similarity threshold, default 0.7
}
