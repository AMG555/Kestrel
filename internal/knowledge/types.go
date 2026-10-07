package knowledge

import (
	"encoding/json"
	"time"
)

// formatTime format化时间为 RFC3339 format，零时间backnullstring
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// KnowledgeItem 知识库项
type KnowledgeItem struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"` // 风险type（file夹名）
	Title     string    `json:"title"`    // title（Filename）
	FilePath  string    `json:"filePath"` // file path
	Content   string    `json:"content"`  // File content
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// KnowledgeItemSummary 知识库项summary（用于list，不包含完整内容）
type KnowledgeItemSummary struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"`
	Title     string    `json:"title"`
	FilePath  string    `json:"filePath"`
	Content   string    `json:"content,omitempty"` // 可选：内容预览（如果提供，通常只包含前 150 字符）
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// MarshalJSON 自定义 JSON 序列化，确保时间format正确
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

// MarshalJSON 自定义 JSON 序列化，确保时间format正确
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

// KnowledgeChunk 知识块（用于向量化）
type KnowledgeChunk struct {
	ID         string    `json:"id"`
	ItemID     string    `json:"itemId"`
	ChunkIndex int       `json:"chunkIndex"`
	ChunkText  string    `json:"chunkText"`
	Embedding  []float32 `json:"-"` // 向量嵌入，不序列化到 JSON
	CreatedAt  time.Time `json:"createdAt"`
}

// RetrievalResult retrieval results
type RetrievalResult struct {
	Chunk      *KnowledgeChunk `json:"chunk"`
	Item       *KnowledgeItem  `json:"item"`
	Similarity float64         `json:"similarity"` // similarity score
	Score      float64         `json:"score"`      // 与 Similarity 相同：余弦相似度
}

// RetrievalLog 检索log
type RetrievalLog struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId,omitempty"`
	MessageID      string    `json:"messageId,omitempty"`
	Query          string    `json:"query"`
	RiskType       string    `json:"riskType,omitempty"`
	RetrievedItems []string  `json:"retrievedItems"` // 检索到的知识项 ID list
	CreatedAt      time.Time `json:"createdAt"`
}

// MarshalJSON 自定义 JSON 序列化，确保时间format正确
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

// CategoryWithItems 分类及其下的知识项（用于按分类paginate）
type CategoryWithItems struct {
	Category  string                  `json:"category"`  // 分类name
	ItemCount int                     `json:"itemCount"` // 该分类下的知识项total
	Items     []*KnowledgeItemSummary `json:"items"`     // 该分类下的Knowledge item list
}

// SearchRequest searchrequest
type SearchRequest struct {
	Query          string  `json:"query"`
	RiskType       string  `json:"riskType,omitempty"`       // 可选：指定风险type
	SubIndexFilter string  `json:"subIndexFilter,omitempty"` // 可选：仅保留 sub_indexes 含该Tags的行（含未打标旧数据）
	TopK           int     `json:"topK,omitempty"`           // back Top-K 结果，默认 5
	Threshold      float64 `json:"threshold,omitempty"`      // 相似度阈value，默认 0.7
}
