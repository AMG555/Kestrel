// Package knowledge provides a RAG (Retrieval-Augmented Generation) pipeline stub.
//
// The pipeline is designed to ground agent recommendations in operator-uploaded
// playbooks and documentation rather than generating novel attack techniques.
//
// Current state: scaffolded interfaces + database-backed storage.
// Future work: plug in a real embedding model (e.g. OpenAI text-embedding-3-small)
// and a vector similarity search (e.g. sqlite-vec or an external store).
package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"kestrel/internal/config"
	"kestrel/internal/database"
)

// Document represents an ingested knowledge base document.
type Document struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	SourcePath  string    `json:"source_path"`
	ContentHash string    `json:"content_hash"`
	ChunkCount  int       `json:"chunk_count"`
	Status      string    `json:"status"` // pending | indexed | failed
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Chunk is a single piece of a document with optional embedding.
type Chunk struct {
	ID         string  `json:"id"`
	DocumentID string  `json:"document_id"`
	Index      int     `json:"chunk_index"`
	Content    string  `json:"content"`
}

// RetrievalResult is a ranked chunk returned from a query.
type RetrievalResult struct {
	Chunk  Chunk   `json:"chunk"`
	Score  float64 `json:"score"`
	Source string  `json:"source"`
}

// Embedder is the interface for embedding text into a vector.
// Real implementations call an LLM embedding API.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// VectorStore is the interface for storing and querying embeddings.
type VectorStore interface {
	Upsert(ctx context.Context, id string, vector []float32, metadata map[string]string) error
	Query(ctx context.Context, vector []float32, topK int) ([]string, []float64, error)
}

// Service manages the knowledge base pipeline.
type Service struct {
	cfg      *config.KnowledgeConfig
	db       *database.DB
	embedder Embedder    // nil when embedding is disabled
	store    VectorStore // nil when vector store is disabled
	logger   *zap.Logger
}

// New creates a knowledge Service.
// If cfg.Enabled is false, all ingest and retrieval methods are no-ops.
func New(cfg *config.KnowledgeConfig, db *database.DB, logger *zap.Logger) *Service {
	return &Service{
		cfg:    cfg,
		db:     db,
		logger: logger,
	}
}

// Enabled returns true if the knowledge base is configured and active.
func (s *Service) Enabled() bool {
	return s.cfg != nil && s.cfg.Enabled
}

// IngestReader reads content from r, chunks it, stores the document and chunks
// in the database, and (when an embedder is configured) indexes embeddings.
func (s *Service) IngestReader(ctx context.Context, title, sourcePath string, r io.Reader) (*Document, error) {
	if !s.Enabled() {
		return nil, fmt.Errorf("knowledge base is not enabled")
	}

	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading document: %w", err)
	}
	content := string(raw)

	// Deduplicate by content hash.
	hash := contentHash(raw)
	var existingID string
	_ = s.db.QueryRow(`SELECT id FROM knowledge_documents WHERE content_hash=?`, hash).Scan(&existingID)
	if existingID != "" {
		s.logger.Info("knowledge: document already indexed", zap.String("hash", hash))
		doc, _ := s.getDocumentByID(existingID)
		return doc, nil
	}

	// Chunk the content.
	chunks := splitChunks(content, s.cfg.ChunkSize, s.cfg.ChunkOverlap)

	now := time.Now().UTC()
	doc := &Document{
		ID:          uuid.New().String(),
		Title:       title,
		SourcePath:  sourcePath,
		ContentHash: hash,
		ChunkCount:  len(chunks),
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Persist document header.
	if _, err := s.db.Exec(`
		INSERT INTO knowledge_documents (id,title,source_path,content_hash,chunk_count,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?)`,
		doc.ID, doc.Title, doc.SourcePath, doc.ContentHash, doc.ChunkCount, doc.Status, now, now,
	); err != nil {
		return nil, fmt.Errorf("persisting document: %w", err)
	}

	// Persist chunks.
	for i, chunk := range chunks {
		chunkID := uuid.New().String()
		if _, err := s.db.Exec(`
			INSERT INTO knowledge_chunks (id,document_id,chunk_index,content,created_at)
			VALUES (?,?,?,?,?)`,
			chunkID, doc.ID, i, chunk, now,
		); err != nil {
			s.logger.Warn("knowledge: failed to persist chunk", zap.Int("index", i), zap.Error(err))
		}

		// Embed and index if an embedder is available.
		if s.embedder != nil && s.store != nil {
			vec, err := s.embedder.Embed(ctx, chunk)
			if err != nil {
				s.logger.Warn("knowledge: embedding failed", zap.Error(err))
				continue
			}
			if err := s.store.Upsert(ctx, chunkID, vec, map[string]string{
				"document_id": doc.ID,
				"title":       doc.Title,
				"chunk_index": fmt.Sprintf("%d", i),
			}); err != nil {
				s.logger.Warn("knowledge: vector upsert failed", zap.Error(err))
			}
		}
	}

	// Mark as indexed.
	status := "indexed"
	if s.embedder == nil {
		status = "indexed_no_vectors" // text search still works
	}
	_, _ = s.db.Exec(`UPDATE knowledge_documents SET status=?, updated_at=? WHERE id=?`, status, time.Now().UTC(), doc.ID)
	doc.Status = status

	s.logger.Info("knowledge: document ingested",
		zap.String("id", doc.ID),
		zap.String("title", doc.Title),
		zap.Int("chunks", len(chunks)),
	)
	return doc, nil
}

// QueryParams carries retrieval parameters.
type QueryParams struct {
	Query   string
	TopK    int
	Rewrite bool // whether to rewrite the query before retrieval
}

// Retrieve returns the most relevant chunks for the given query.
// When no embedder is configured, falls back to keyword search over chunk content.
func (s *Service) Retrieve(ctx context.Context, p QueryParams) ([]RetrievalResult, error) {
	if !s.Enabled() {
		return nil, nil
	}

	topK := p.TopK
	if topK <= 0 {
		topK = s.cfg.TopK
	}
	if topK <= 0 {
		topK = 5
	}

	// Vector retrieval path (when embedder is available).
	if s.embedder != nil && s.store != nil {
		query := p.Query
		if p.Rewrite {
			query = rewriteQuery(query)
		}
		vec, err := s.embedder.Embed(ctx, query)
		if err != nil {
			s.logger.Warn("knowledge: query embedding failed — falling back to keyword search", zap.Error(err))
		} else {
			ids, scores, err := s.store.Query(ctx, vec, topK)
			if err == nil {
				return s.hydrateChunks(ids, scores)
			}
			s.logger.Warn("knowledge: vector query failed — falling back to keyword search", zap.Error(err))
		}
	}

	// Keyword fallback: simple LIKE search over chunk content.
	return s.keywordSearch(p.Query, topK)
}

// keywordSearch performs a simple LIKE search over chunk content.
func (s *Service) keywordSearch(query string, limit int) ([]RetrievalResult, error) {
	// Build OR conditions for each word in the query.
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil, nil
	}

	conditions := make([]string, len(words))
	args := make([]interface{}, len(words))
	for i, w := range words {
		conditions[i] = "content LIKE ?"
		args[i] = "%" + w + "%"
	}
	args = append(args, limit)

	rows, err := s.db.Query(
		`SELECT kc.id, kc.document_id, kc.chunk_index, kc.content, kd.title
		 FROM knowledge_chunks kc
		 JOIN knowledge_documents kd ON kd.id = kc.document_id
		 WHERE `+strings.Join(conditions, " OR ")+`
		 LIMIT ?`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RetrievalResult
	for rows.Next() {
		var c Chunk
		var docTitle string
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.Index, &c.Content, &docTitle); err != nil {
			continue
		}
		results = append(results, RetrievalResult{
			Chunk:  c,
			Score:  0.5, // Placeholder relevance score for keyword matches.
			Source: docTitle,
		})
	}
	return results, rows.Err()
}

// hydrateChunks resolves chunk IDs returned from the vector store to full chunk records.
func (s *Service) hydrateChunks(ids []string, scores []float64) ([]RetrievalResult, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	results := make([]RetrievalResult, 0, len(ids))
	for i, id := range ids {
		var c Chunk
		var docTitle string
		err := s.db.QueryRow(
			`SELECT kc.id, kc.document_id, kc.chunk_index, kc.content, kd.title
			 FROM knowledge_chunks kc
			 JOIN knowledge_documents kd ON kd.id = kc.document_id
			 WHERE kc.id=?`, id,
		).Scan(&c.ID, &c.DocumentID, &c.Index, &c.Content, &docTitle)
		if err != nil {
			continue
		}
		score := 0.0
		if i < len(scores) {
			score = scores[i]
		}
		results = append(results, RetrievalResult{Chunk: c, Score: score, Source: docTitle})
	}
	return results, nil
}

// ListDocuments returns all knowledge base documents.
func (s *Service) ListDocuments() ([]*Document, error) {
	rows, err := s.db.Query(
		`SELECT id,title,source_path,content_hash,chunk_count,status,created_at,updated_at
		 FROM knowledge_documents ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []*Document
	for rows.Next() {
		d := &Document{}
		if err := rows.Scan(&d.ID, &d.Title, &d.SourcePath, &d.ContentHash, &d.ChunkCount, &d.Status, &d.CreatedAt, &d.UpdatedAt); err != nil {
			continue
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}

// DeleteDocument removes a document and its chunks.
func (s *Service) DeleteDocument(id string) error {
	_, err := s.db.Exec(`DELETE FROM knowledge_documents WHERE id=?`, id)
	return err
}

// getDocumentByID retrieves a single document header.
func (s *Service) getDocumentByID(id string) (*Document, error) {
	d := &Document{}
	err := s.db.QueryRow(
		`SELECT id,title,source_path,content_hash,chunk_count,status,created_at,updated_at
		 FROM knowledge_documents WHERE id=?`, id,
	).Scan(&d.ID, &d.Title, &d.SourcePath, &d.ContentHash, &d.ChunkCount, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return d, nil
}

// --- Helpers ---

// splitChunks splits text into overlapping chunks of approximately chunkSize runes.
func splitChunks(text string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 512
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4
	}

	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	var chunks []string
	step := chunkSize - overlap
	if step <= 0 {
		step = chunkSize
	}

	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[start:end]))
		if end == len(runes) {
			break
		}
	}
	return chunks
}

// contentHash returns a hex SHA-256 of raw content.
func contentHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// rewriteQuery applies a simple query expansion to improve recall.
// A real implementation would call an LLM for HyDE or step-back prompting.
func rewriteQuery(q string) string {
	// Stub: return original query unchanged.
	return q
}
