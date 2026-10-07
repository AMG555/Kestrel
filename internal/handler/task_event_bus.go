package handler

import "sync"

// TaskEventBus mirrors events from the main SSE connection to later-subscribing clients (e.g. after page refresh, or when HITL approval is granted and the client needs to continue receiving events).
// Each payload is a complete SSE line: "data: {...}\n\n".
type TaskEventBus struct {
	mu   sync.RWMutex
	subs map[string]map[*taskEventSub]struct{}
}

type taskEventSub struct {
	mu     sync.Mutex
	ch     chan []byte
	closed bool
}

func (s *taskEventSub) sendNonBlocking(line []byte) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	select {
	case s.ch <- line:
		return true
	default:
		return false
	}
}

func (s *taskEventSub) closeOnce() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	close(s.ch)
}

func NewTaskEventBus() *TaskEventBus {
	return &TaskEventBus{
		subs: make(map[string]map[*taskEventSub]struct{}),
	}
}

// Subscribe registers a subscription; call Unsubscribe when cancelling.
func (b *TaskEventBus) Subscribe(conversationID string) (sub *taskEventSub, ch <-chan []byte) {
	chBuf := make(chan []byte, 256)
	sub = &taskEventSub{ch: chBuf}
	b.mu.Lock()
	if b.subs[conversationID] == nil {
		b.subs[conversationID] = make(map[*taskEventSub]struct{})
	}
	b.subs[conversationID][sub] = struct{}{}
	b.mu.Unlock()
	return sub, chBuf
}

func (b *TaskEventBus) Unsubscribe(conversationID string, sub *taskEventSub) {
	if sub == nil {
		return
	}
	b.mu.Lock()
	m, ok := b.subs[conversationID]
	if !ok {
		b.mu.Unlock()
		return
	}
	delete(m, sub)
	if len(m) == 0 {
		delete(b.subs, conversationID)
	}
	b.mu.Unlock()
	sub.closeOnce()
}

// Publish delivers non-blocking; slow consumers may drop frames (HITL scenarios use the latest status, so dropped frames are acceptable).
func (b *TaskEventBus) Publish(conversationID string, line []byte) {
	if b == nil || conversationID == "" || len(line) == 0 {
		return
	}
	b.mu.RLock()
	m := b.subs[conversationID]
	subs := make([]*taskEventSub, 0, len(m))
	for s := range m {
		subs = append(subs, s)
	}
	b.mu.RUnlock()

	cp := append([]byte(nil), line...)
	for _, s := range subs {
		s.sendNonBlocking(cp)
	}
}

// CloseConversation closes all subscription channels for a conversation when the task ends.
func (b *TaskEventBus) CloseConversation(conversationID string) {
	if b == nil || conversationID == "" {
		return
	}
	b.mu.Lock()
	m := b.subs[conversationID]
	delete(b.subs, conversationID)
	b.mu.Unlock()
	for sub := range m {
		sub.closeOnce()
	}
}
