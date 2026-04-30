package sse

import (
	"context"
	"sync"
	"sync/atomic"
)

type Event struct {
	ID    string
	Type  string // maps to SSE "event:" field
	Data  []byte
	Retry int // optional reconnect hint in ms
}

// client holds a subscriber's send channel and context.
type client struct {
	ch     chan Event
	cancel context.CancelFunc
}

// Broker manages pub/sub for typed SSE events.
type Broker struct {
	mu      sync.RWMutex
	clients map[uint64]*client
	nextID  atomic.Uint64
	buf     int // channel buffer size per client
}

func NewBroker(bufSize int) *Broker {
	if bufSize <= 0 {
		bufSize = 16
	}
	return &Broker{
		clients: make(map[uint64]*client),
		buf:     bufSize,
	}
}

// subscribe registers a client and returns its event channel + cleanup func.
func (b *Broker) Subscribe(ctx context.Context) (uint64, <-chan Event, context.CancelFunc) {
	id := b.nextID.Add(1)
	ctx, cancel := context.WithCancel(ctx)
	c := &client{
		ch:     make(chan Event, b.buf),
		cancel: cancel,
}

	b.mu.Lock()
	b.clients[id] = c
	b.mu.Unlock()

	return id, c.ch, func() {
		cancel()

		b.mu.Lock()
		if client, ok := b.clients[id]; ok {
			delete(b.clients, id)
			close(client.ch)
		}
		b.mu.Unlock()
	}
}

// Publish fans an event out to all connected clients.
// Slow clients are skipped (non-blocking send).
func (b *Broker) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, c := range b.clients {
		select {
		case c.ch <- e:
		default: // drop on full buffer; client is too slow
		}
	}
}

// Count returns the number of active subscribers.
func (b *Broker) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.clients)
}

func (b *Broker) PublishTo(clientID uint64, e Event) bool {
	b.mu.RLock()
	client, ok := b.clients[clientID]
	b.mu.RUnlock()

	if !ok {
		return false
	}

	select {
	case client.ch <- e:
		return true
	default:
		return false // client too slow
	}
}

func (b *Broker) PublishMany(clientIDs []uint64, e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, id := range clientIDs {
		client, ok := b.clients[id]
		if !ok {
			continue
		}

		select {
		case client.ch <- e:
		default:
		}
	}
}
