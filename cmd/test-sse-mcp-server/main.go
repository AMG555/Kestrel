package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

const ProtocolVersion = "2024-11-05"

type Message struct {
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	Version string          `json:"jsonrpc,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type SSEServer struct {
	mu      sync.RWMutex
	clients map[string]chan []byte
}

func NewSSEServer() *SSEServer {
	return &SSEServer{clients: make(map[string]chan []byte)}
}

func (s *SSEServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientID := uuid.New().String()
	ch := make(chan []byte, 10)

	s.mu.Lock()
	s.clients[clientID] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, clientID)
		close(ch)
		s.mu.Unlock()
	}()

	fmt.Fprintf(w, "event: message\ndata: {\"type\":\"ready\",\"status\":\"ok\"}\n\n")
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (s *SSEServer) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var msg Message
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	res := s.process(&msg)
	b, _ := json.Marshal(res)

	s.mu.RLock()
	for _, ch := range s.clients {
		select {
		case ch <- b:
		default:
		}
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

func (s *SSEServer) process(msg *Message) *Message {
	switch msg.Method {
	case "initialize":
		res, _ := json.Marshal(map[string]interface{}{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]interface{}{"tools": map[string]bool{"listChanged": true}},
			"serverInfo":      map[string]string{"name": "Kestrel Test SSE Server", "version": "1.0.0"},
		})
		return &Message{ID: msg.ID, Version: "2.0", Result: res}
	case "tools/list":
		res, _ := json.Marshal(map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "ping_target",
					"description": "Echo and connectivity verification test tool",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"target": map[string]string{"type": "string"},
						},
						"required": []string{"target"},
					},
				},
			},
		})
		return &Message{ID: msg.ID, Version: "2.0", Result: res}
	case "tools/call":
		res, _ := json.Marshal(map[string]interface{}{
			"content": []map[string]string{
				{"type": "text", "text": "Target response verified successfully"},
			},
		})
		return &Message{ID: msg.ID, Version: "2.0", Result: res}
	default:
		return &Message{
			ID:      msg.ID,
			Version: "2.0",
			Error:   &Error{Code: -32601, Message: "Method not found"},
		}
	}
}

func main() {
	srv := NewSSEServer()
	http.HandleFunc("/sse", srv.handleSSE)
	http.HandleFunc("/message", srv.handleMessage)

	port := ":8082"
	log.Printf("Starting Kestrel SSE MCP test server on %s", port)
	log.Printf("SSE endpoint: http://127.0.0.1%s/sse", port)
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}
