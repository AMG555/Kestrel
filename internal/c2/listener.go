package c2

import (
	"strings"
	"sync"

	"kestrel/internal/database"

	"go.uber.org/zap"
)

// Listener is an abstract interface for listeners: each transport type (TCP/HTTP/HTTPS/WS/DNS) implements this interface;
// the Manager does not know the concrete implementation details; it creates instances through the ListenerRegistry factory.
type Listener interface {
	// Type returns the type string of the current listener (e.g. "tcp_reverse")
	Type() string
	// Start begins listening; should return ErrPortInUse if the port is already in use.
	Start() error
	// Stop stops listening and releases all associated goroutines (must not panic).
	Stop() error
}

// ListenerCreationCtx is the context received by the factory when initialising a listener.
type ListenerCreationCtx struct {
	Listener *database.C2Listener
	Config   *ListenerConfig
	Manager  *Manager
	Logger   *zap.Logger
}

// ListenerFactory is the factory for creating Listener instances; the returned instance has not yet been started.
type ListenerFactory func(ctx ListenerCreationCtx) (Listener, error)

// ListenerRegistry is the registry of type → factory mappings. Concrete implementations are registered in internal/app at startup;
// mock factories can also be injected in tests.
type ListenerRegistry struct {
	mu        sync.RWMutex
	factories map[string]ListenerFactory
}

// NewListenerRegistry creates an empty registry.
func NewListenerRegistry() *ListenerRegistry {
	return &ListenerRegistry{factories: make(map[string]ListenerFactory)}
}

// Register registers a listener factory for the given type name.
func (r *ListenerRegistry) Register(typeName string, f ListenerFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[strings.ToLower(strings.TrimSpace(typeName))] = f
}

// Get retrieves a factory; returns nil if not registered.
func (r *ListenerRegistry) Get(typeName string) ListenerFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.factories[strings.ToLower(strings.TrimSpace(typeName))]
}

// RegisteredTypes returns the list of registered listener types, for frontend enumeration.
func (r *ListenerRegistry) RegisteredTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for k := range r.factories {
		out = append(out, k)
	}
	return out
}
