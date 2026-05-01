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

// connection holds a subscriber's send channel and context.
type Connection struct {
	id     uint64
	userID string
	ch     chan Event
	cancel context.CancelFunc
}

// Broker manages pub/sub for typed SSE events.
type Broker struct {
	mu sync.RWMutex

	// connectionID -> connection
	connections map[uint64]*Connection

	nextID atomic.Uint64
	buf    int // channel buffer size per client
}

func NewBroker(bufSize int) *Broker {
	if bufSize <= 0 {
		bufSize = 32
	}
	return &Broker{
		connections: make(map[uint64]*Connection),
		buf:         bufSize,
	}
}

// subscribe registers a client and returns its event channel + cleanup func.
func (b *Broker) Subscribe(ctx context.Context, userID string) (uint64, <-chan Event, context.CancelFunc) {
	id := b.nextID.Add(1)
	ctx, cancel := context.WithCancel(ctx)
	c := &Connection{
		id:     id,
		userID: userID,
		ch:     make(chan Event, b.buf),
		cancel: cancel,
	}
	b.mu.Lock()
	b.connections[id] = c
	b.mu.Unlock()

	return id, c.ch, func() {
		cancel()
		b.removeConnection(id)
	}
}

// Publish fans an event out to all connected clients.
// Slow clients are skipped (non-blocking send).
func (b *Broker) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, c := range b.connections {
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
	return len(b.connections)
}

func (b *Broker) PublishToConnection(connectionId uint64, e Event) bool {
	b.mu.RLock()
	connection, ok := b.connections[connectionId]
	b.mu.RUnlock()

	if !ok {
		return false
	}

	select {
	case connection.ch <- e:
		return true
	default:
		return false // client too slow
	}
}

func (b *Broker) PublishToConnections(connectionIds []uint64, e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, id := range connectionIds {
		client, ok := b.connections[id]
		if !ok {
			continue
		}

		select {
		case client.ch <- e:
		default:
		}
	}
}

func (b *Broker) PublishToUser(userID string, e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, conn := range b.connections {
		if conn.userID != userID {
			continue
		}

		select {
		case conn.ch <- e:
		default:
		}
	}
}

func (b *Broker) PublishToUsers(userIDs []string, e Event) {
	// make sure we have unique userIds
	userSet := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		userSet[userID] = struct{}{}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, conn := range b.connections {
		if _, ok := userSet[conn.userID]; !ok {
			continue
		}

		select {
		case conn.ch <- e:
		default:
		}
	}
}

func (b *Broker) removeConnection(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	conn, ok := b.connections[id]
	if !ok {
		return
	}

	delete(b.connections, id)
	close(conn.ch)
}
