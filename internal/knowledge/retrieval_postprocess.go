package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"kestrel/internal/config"

	"github.com/cloudwego/eino/schema"
	"github.com/pkoukk/tiktoken-go"
)

// postRetrieveMaxPrefetchCap limits the maximum vector candidates per query to avoid excessive full-table scans from misconfiguration.
const postRetrieveMaxPrefetchCap = 200

// DocumentReranker is the fine-grained reranker (HTTP dashscope / Cohere-compatible API), injected by [WireRetrieverPipeline].
type DocumentReranker interface {
	Rerank(ctx context.Context, query string, docs []*schema.Document) ([]*schema.Document, error)
}

// NopDocumentReranker is a placeholder implementation for use in tests or when reranking is disabled.
type NopDocumentReranker struct{}

// Rerank implements [DocumentReranker] as no-op.
func (NopDocumentReranker) Rerank(_ context.Context, _ string, docs []*schema.Document) ([]*schema.Document, error) {
	return docs, nil
}

var tiktokenEncMu sync.Mutex
var tiktokenEncCache = map[string]*tiktoken.Tiktoken{}

func encodingForTokenizerModel(model string) (*tiktoken.Tiktoken, error) {
	m := strings.TrimSpace(model)
	if m == "" {
		m = "gpt-4"
	}
	tiktokenEncMu.Lock()
	defer tiktokenEncMu.Unlock()
	if enc, ok := tiktokenEncCache[m]; ok {
		return enc, nil
	}
	enc, err := tiktoken.EncodingForModel(m)
	if err != nil {
		enc, err = tiktoken.GetEncoding("cl100k_base")
		if err != nil {
			return nil, err
		}
	}
	tiktokenEncCache[m] = enc
	return enc, nil
}

func countDocTokens(text, model string) (int, error) {
	enc, err := encodingForTokenizerModel(model)
	if err != nil {
		return 0, err
	}
	toks := enc.Encode(text, nil, nil)
	return len(toks), nil
}

// normalizeContentFingerprintKey computes a deduplication key: trims and collapses whitespace
// (case is preserved to avoid merging code snippets that differ only in case).
func normalizeContentFingerprintKey(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

func contentNormKey(d *schema.Document) string {
	if d == nil {
		return ""
	}
	n := normalizeContentFingerprintKey(d.Content)
	if n == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(n))
	return hex.EncodeToString(sum[:])
}

// dedupeByNormalizedContent deduplicates by normalized content, keeping the first occurrence in retrieval order
// (only one entry is kept per identical body).
func dedupeByNormalizedContent(docs []*schema.Document) []*schema.Document {
	if len(docs) < 2 {
		return docs
	}
	seen := make(map[string]struct{}, len(docs))
	out := make([]*schema.Document, 0, len(docs))
	for _, d := range docs {
		if d == nil {
			continue
		}
		k := contentNormKey(d)
		if k == "" {
			out = append(out, d)
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, d)
	}
	return out
}

// truncateDocumentsByBudget retains documents in retrieval order until the character or token budget
// (whichever is enabled) is exceeded.
func truncateDocumentsByBudget(docs []*schema.Document, maxRunes, maxTokens int, tokenModel string) ([]*schema.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}
	unlimitedChars := maxRunes <= 0
	unlimitedTok := maxTokens <= 0
	if unlimitedChars && unlimitedTok {
		return docs, nil
	}

	remRunes := maxRunes
	remTok := maxTokens
	out := make([]*schema.Document, 0, len(docs))

	for _, d := range docs {
		if d == nil || strings.TrimSpace(d.Content) == "" {
			continue
		}
		runes := utf8.RuneCountInString(d.Content)
		if !unlimitedChars && runes > remRunes {
			break
		}
		var tok int
		var err error
		if !unlimitedTok {
			tok, err = countDocTokens(d.Content, tokenModel)
			if err != nil {
				return nil, fmt.Errorf("token count: %w", err)
			}
			if tok > remTok {
				break
			}
		}
		out = append(out, d)
		if !unlimitedChars {
			remRunes -= runes
		}
		if !unlimitedTok {
			remTok -= tok
		}
	}
	return out, nil
}

// EffectivePrefetchTopK computes the number of vector candidates per MultiQuery variant
// (used for fusion / reranking / post-processing).
func EffectivePrefetchTopK(topK int, po *config.PostRetrieveConfig) int {
	if topK < 1 {
		topK = 5
	}
	fetch := topK * 4
	if fetch < 20 {
		fetch = 20
	}
	if po != nil && po.PrefetchTopK > 0 {
		fetch = po.PrefetchTopK
	}
	if fetch > postRetrieveMaxPrefetchCap {
		fetch = postRetrieveMaxPrefetchCap
	}
	return fetch
}

// ApplyPostRetrieve applies post-retrieval processing: normalized-content deduplication → budget truncation → final TopK
// (fine-grained reranking is already done in the pipeline).
func ApplyPostRetrieve(docs []*schema.Document, po *config.PostRetrieveConfig, tokenModel string, finalTopK int) ([]*schema.Document, error) {
	if finalTopK < 1 {
		finalTopK = 5
	}
	if len(docs) == 0 {
		return docs, nil
	}

	maxChars := 0
	maxTok := 0
	if po != nil {
		maxChars = po.MaxContextChars
		maxTok = po.MaxContextTokens
	}

	out := dedupeByNormalizedContent(docs)

	var err error
	out, err = truncateDocumentsByBudget(out, maxChars, maxTok, tokenModel)
	if err != nil {
		return nil, err
	}

	if len(out) > finalTopK {
		out = out[:finalTopK]
	}
	return out, nil
}
