package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"kestrel/internal/database"
	"kestrel/internal/llm"
)

// OSINTHandler provides cyberspace search and intelligence discovery.
type OSINTHandler struct {
	db     *database.DB
	llm    *llm.Manager
	logger *zap.Logger
	client *http.Client
}

// NewOSINTHandler creates an OSINTHandler.
func NewOSINTHandler(db *database.DB, llmMgr *llm.Manager, logger *zap.Logger) *OSINTHandler {
	return &OSINTHandler{
		db:     db,
		llm:    llmMgr,
		logger: logger,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type osintParseReq struct {
	Provider string `json:"provider"`
	Text     string `json:"text" binding:"required"`
}

type osintSearchReq struct {
	Provider string `json:"provider"` // fofa | shodan | zoomeye | quake
	Query    string `json:"query" binding:"required"`
	Page     int    `json:"page"`
	Size     int    `json:"size"`
}

type osintImportReq struct {
	ProjectID string                   `json:"project_id"`
	Results   []map[string]interface{} `json:"results" binding:"required"`
}

// ParseNLQuery handles POST /api/osint/parse.
func (h *OSINTHandler) ParseNLQuery(c *gin.Context) {
	var req osintParseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}

	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = "fofa"
	}

	prompt := fmt.Sprintf(
		"Convert this natural language query into valid %s search syntax: %q. Output ONLY the query string, nothing else.",
		provider, req.Text,
	)

	generatedQuery := ""
	if h.llm != nil {
		resp, err := h.llm.Default().Complete(c.Request.Context(), llm.CompletionRequest{
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "You are a cyberspace asset search expert. Convert natural language into query syntax."},
				{Role: llm.RoleUser, Content: prompt},
			},
		})
		if err == nil {
			generatedQuery = strings.TrimSpace(resp.Content)
		}
	}

	// Fallback heuristic if LLM unconfigured
	if generatedQuery == "" {
		text := req.Text
		if strings.Contains(strings.ToLower(text), "apache") {
			generatedQuery = `app="Apache"`
		} else if strings.Contains(strings.ToLower(text), "nginx") {
			generatedQuery = `app="nginx"`
		} else {
			generatedQuery = fmt.Sprintf(`title="%s"`, text)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"provider": provider,
		"query":    generatedQuery,
	})
}

// Search handles POST /api/osint/search.
func (h *OSINTHandler) Search(c *gin.Context) {
	var req osintSearchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	if provider == "" {
		provider = "fofa"
	}

	size := req.Size
	if size <= 0 || size > 100 {
		size = 20
	}
	page := req.Page
	if page <= 0 {
		page = 1
	}

	// Check environment keys
	fofaKey := os.Getenv("FOFA_KEY")
	shodanKey := os.Getenv("SHODAN_API_KEY")

	if provider == "fofa" && fofaKey != "" {
		qB64 := base64.StdEncoding.EncodeToString([]byte(req.Query))
		apiURL := fmt.Sprintf("https://fofa.info/api/v1/search/all?key=%s&qbase64=%s&page=%d&size=%d&fields=host,ip,port,protocol,title,country_name,server",
			fofaKey, qB64, page, size)
		resp, err := h.client.Get(apiURL)
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var fofaResp struct {
				Error   bool            `json:"error"`
				ErrMsg  string          `json:"errmsg"`
				Total   int             `json:"total"`
				Results [][]interface{} `json:"results"`
			}
			if json.Unmarshal(body, &fofaResp) == nil && !fofaResp.Error {
				var items []map[string]interface{}
				for _, r := range fofaResp.Results {
					m := map[string]interface{}{}
					if len(r) > 0 { m["host"] = r[0] }
					if len(r) > 1 { m["ip"] = r[1] }
					if len(r) > 2 { m["port"] = r[2] }
					if len(r) > 3 { m["protocol"] = r[3] }
					if len(r) > 4 { m["title"] = r[4] }
					if len(r) > 5 { m["country"] = r[5] }
					if len(r) > 6 { m["server"] = r[6] }
					items = append(items, m)
				}
				c.JSON(http.StatusOK, gin.H{
					"provider": provider,
					"query":    req.Query,
					"total":    fofaResp.Total,
					"results":  items,
				})
				return
			}
		}
	} else if provider == "shodan" && shodanKey != "" {
		apiURL := fmt.Sprintf("https://api.shodan.io/shodan/host/search?key=%s&query=%s&page=%d",
			shodanKey, url.QueryEscape(req.Query), page)
		resp, err := h.client.Get(apiURL)
		if err == nil {
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			var shodanResp struct {
				Total   int `json:"total"`
				Matches []struct {
					IPStr string `json:"ip_str"`
					Port  int    `json:"port"`
					Hostnames []string `json:"hostnames"`
					Product string `json:"product"`
					Location struct {
						CountryName string `json:"country_name"`
					} `json:"location"`
				} `json:"matches"`
			}
			if json.Unmarshal(body, &shodanResp) == nil {
				var items []map[string]interface{}
				for _, m := range shodanResp.Matches {
					host := m.IPStr
					if len(m.Hostnames) > 0 {
						host = m.Hostnames[0]
					}
					items = append(items, map[string]interface{}{
						"host":     host,
						"ip":       m.IPStr,
						"port":     m.Port,
						"protocol": "tcp",
						"server":   m.Product,
						"country":  m.Location.CountryName,
					})
				}
				c.JSON(http.StatusOK, gin.H{
					"provider": provider,
					"query":    req.Query,
					"total":    shodanResp.Total,
					"results":  items,
				})
				return
			}
		}
	}

	// Demonstration sandbox fallback when external API keys are not supplied
	demoItems := []map[string]interface{}{
		{
			"host":     "target-app1.internal.local",
			"ip":       "192.168.10.45",
			"port":     443,
			"protocol": "https",
			"title":    "Enterprise Access Portal",
			"country":  "Internal Lab",
			"server":   "nginx/1.24.0",
		},
		{
			"host":     "api.target-app2.internal.local",
			"ip":       "192.168.10.50",
			"port":     8080,
			"protocol": "http",
			"title":    "Swagger UI - Gateway Service",
			"country":  "Internal Lab",
			"server":   "Apache-Coyote/1.1",
		},
	}

	c.JSON(http.StatusOK, gin.H{
		"provider": provider,
		"query":    req.Query,
		"total":    len(demoItems),
		"results":  demoItems,
		"notice":   "Operating with simulated asset intelligence. Set FOFA_KEY or SHODAN_API_KEY for live feed.",
	})
}

// ImportAssets handles POST /api/osint/import-assets.
func (h *OSINTHandler) ImportAssets(c *gin.Context) {
	var req osintImportReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "results array is required"})
		return
	}

	importedCount := 0
	for _, r := range req.Results {
		host, _ := r["host"].(string)
		ip, _ := r["ip"].(string)
		if host == "" && ip == "" {
			continue
		}
		target := host
		if target == "" {
			target = ip
		}
		port := 0
		if pVal, ok := r["port"]; ok {
			switch p := pVal.(type) {
			case float64:
				port = int(p)
			case int:
				port = p
			case string:
				port, _ = strconv.Atoi(p)
			}
		}

		proto, _ := r["protocol"].(string)
		title, _ := r["title"].(string)
		server, _ := r["server"].(string)

		_, err := h.db.UpsertAsset(&database.Asset{
			ProjectID: req.ProjectID,
			Host:      target,
			IP:        ip,
			Port:      port,
			Protocol:  proto,
			Title:     title,
			Service:   server,
			Status:    "active",
			Source:    "osint",
		})
		if err == nil {
			importedCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"imported_count": importedCount,
		"message":        fmt.Sprintf("Successfully imported %d assets into inventory", importedCount),
	})
}
