package c2

import (
	"sync"
	"sync/atomic"
	"time"
)

// Event is the unit of transmission inside EventBus, and is a "real-time projection" of database.C2Event.
// The difference is:
//   - The database table saves all history, for audit and paginated listing;
//   - EventBus only caches the most recent N entries, for real-time SSE/WS push to online subscribers.
type Event struct {
	ID        string                 `json:"id"`
	Level     string                 `json:"level"`
	Category  string                 `json:"category"`
	SessionID string                 `json:"sessionId,omitempty"`
	TaskID    string                 `json:"taskId,omitempty"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
}

// EventBus is a simple in-memory broadcast bus.
// Design notes:
//   - Multiple subscribers: each subscriber has an independent buffered channel; slow consumers do not block the publisher;
//   - Drop on full: the publisher never blocks; drops silently when the channel is full to avoid blocking the listener accept loop / beacon handler;
//   - Global filter: subscribers can be scoped by SessionID/Category on subscribe, reducing CPU;
//   - Close-safe: after Close(), all subscriber channels are closed to prevent goroutine leaks.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]*Subscription
	closed      bool
}

// Subscription is a subscription handle.
type Subscription struct {
	ID         string
	Ch         chan *Event
	SessionID  string // empty string means no restriction
	Category   string // empty string means no restriction
	Levels     map[string]struct{}
	dropCount  atomic.Int64
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{subscribers: make(map[string]*Subscription)}
}

// Subscribe registers a subscriber and returns a Subscription; the caller is responsible for calling Unsubscribe later.
//   - bufferSize: per-subscriber channel capacity, recommended 64–256;
//   - sessionFilter / categoryFilter: empty string means no restriction;
//   - levelFilter: e.g. []string{"warn","critical"}; nil/empty means receive all.
func (b *EventBus) Subscribe(id string, bufferSize int, sessionFilter, categoryFilter string, levelFilter []string) *Subscription {
	if bufferSize <= 0 {
		bufferSize = 128
	}
	sub := &Subscription{
		ID:        id,
		Ch:        make(chan *Event, bufferSize),
		SessionID: sessionFilter,
		Category:  categoryFilter,
	}
	if len(levelFilter) > 0 {
		sub.Levels = make(map[string]struct{}, len(levelFilter))
		for _, l := range levelFilter {
			sub.Levels[l] = struct{}{}
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		close(sub.Ch)
		return sub
	}
	b.subscribers[id] = sub
	return sub
}

// Unsubscribe unregisters a subscriber and closes its channel.
func (b *EventBus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if sub, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(sub.Ch)
	}
}

// Publish broadcasts an event to all matching subscribers; non-blocking, silently drops if the channel is full.
func (b *EventBus) Publish(e *Event) {
	if e == nil {
		return
	}
	b.mu.RLock()
	subs := make([]*Subscription, 0, len(b.subscribers))
	for _, s := range b.subscribers {
		if s.matches(e) {
			subs = append(subs, s)
		}
	}
	closed := b.closed
	b.mu.RUnlock()
	if closed {
		return
	}
	for _, s := range subs {
		select {
		case s.Ch <- e:
		default:
			s.dropCount.Add(1)
		}
	}
}

// Close shuts down the bus and stops all subscriptions.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for id, s := range b.subscribers {
		close(s.Ch)
		delete(b.subscribers, id)
	}
}

func (s *Subscription) matches(e *Event) bool {
	if s.SessionID != "" && e.SessionID != s.SessionID {
		return false
	}
	if s.Category != "" && e.Category != s.Category {
		return false
	}
	if len(s.Levels) > 0 {
		if _, ok := s.Levels[e.Level]; !ok {
			return false
		}
	}
	return true
}
