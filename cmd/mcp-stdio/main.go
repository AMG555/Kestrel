// Package main implements a standalone stdio MCP server for Kestrel.
// External clients (such as Claude Desktop or subagents) can connect over stdio to invoke Kestrel recon tools.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap"
	"kestrel/internal/config"
	"kestrel/internal/logger"
	"kestrel/internal/mcp"
	"kestrel/internal/vision"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	log, err := logger.New("warn", "stderr")
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger initialization failed: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	cfg, _ := config.Load(*configPath)
	if cfg == nil {
		cfg = &config.Config{}
	}

	tgCfg := cfg.EffectiveToolGuard()
	registry := mcp.NewRegistryWithGuard(&cfg.MCP, &tgCfg, log)
	vision.RegisterVisionTool(registry)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendError(nil, -32700, "Parse error")
			continue
		}

		handleMethod(context.Background(), registry, req, log)
	}
}

func handleMethod(ctx context.Context, reg *mcp.Registry, req jsonRPCRequest, log *zap.Logger) {
	switch req.Method {
	case "initialize":
		sendResult(req.ID, map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]interface{}{
				"name":    "kestrel-mcp-stdio",
				"version": "1.0.0",
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		})

	case "tools/list":
		tools := reg.ListTools()
		type toolItem struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		}
		list := make([]toolItem, len(tools))
		for i, t := range tools {
			list[i] = toolItem{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.Parameters,
			}
		}
		sendResult(req.ID, map[string]interface{}{"tools": list})

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			sendError(req.ID, -32602, "Invalid params")
			return
		}

		res, err := reg.Execute(ctx, params.Name, params.Arguments, nil)
		if err != nil {
			sendError(req.ID, -32000, err.Error())
			return
		}

		sendResult(req.ID, map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": res.Output},
			},
		})

	default:
		sendError(req.ID, -32601, "Method not found")
	}
}

func sendResult(id interface{}, result interface{}) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Printf("%s\n", data)
}

func sendError(id interface{}, code int, msg string) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: map[string]interface{}{
			"code":    code,
			"message": msg,
		},
	}
	data, _ := json.Marshal(resp)
	fmt.Printf("%s\n", data)
}
