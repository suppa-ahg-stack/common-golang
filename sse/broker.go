package sse

import (
	"context"
	"sync"
	"sync/atomic"
)

// Event is a typed SSE message.
type Event struct {
	ID    string // maps to SSE "id:" field
	Type  string // maps to SSE "event:" field
	Data  []byte
	Retry int // optional reconnect hint in ms
}

// Connection holds a subscriber's send channel and routing metadata.
type Connection struct {
	id          uint64
	sessionID   string
	userID      string
	page        string
	selectors   map[string]bool
	fragmentIDs map[string]bool
	ch          chan Event
	cancel      context.CancelFunc
}

// PageContext returns the current page and fragment sets for a connection.
func (c *Connection) PageContext() (page string, selectors, fragmentIDs map[string]bool) {
	return c.page, c.selectors, c.fragmentIDs
}

// HasPageContext reports whether the connection has received an explicit page
// context. When false, fragment filtering should fall back to delivery so that
// events are not silently lost before the first page update arrives.
func (c *Connection) HasPageContext() bool {
	return c.page != ""
}

// ConnectionID returns the unique connection identifier.
func (c *Connection) ConnectionID() uint64 {
	return c.id
}

// SessionID returns the session identifier for the connection.
func (c *Connection) SessionID() string {
	return c.sessionID
}

// UserID returns the user identifier for the connection.
func (c *Connection) UserID() string {
	return c.userID
}

// PublishOptions configures user-scoped publishing.
type PublishOptions struct {
	// ActiveSessionIDs restricts publishing to sessions whose ID is in this set.
	// A nil map disables the active-session filter.
	ActiveSessionIDs map[string]bool
	// Filter is an optional per-connection predicate.
	Filter func(*Connection) bool
}

// Broker manages pub/sub for typed SSE events with user/session/connection routing.
type Broker struct {
	mu sync.RWMutex

	// connectionID -> connection
	connections map[uint64]*Connection

	// sessionID -> set of connection IDs
	sessions map[string]map[uint64]bool

	// userID -> set of session IDs
	users map[string]map[string]bool

	nextID atomic.Uint64
	buf    int // channel buffer size per client
}

// NewBroker creates a broker with the given per-connection channel buffer size.
func NewBroker(bufSize int) *Broker {
	if bufSize <= 0 {
		bufSize = 32
	}
	return &Broker{
		connections: make(map[uint64]*Connection),
		sessions:    make(map[string]map[uint64]bool),
		users:       make(map[string]map[string]bool),
		buf:         bufSize,
	}
}

// Subscribe registers a client keyed by userID.
// The provided userID is also used as the sessionID.
// It is kept for backward compatibility; prefer SubscribeWithIDs.
func (b *Broker) Subscribe(ctx context.Context, userID string) (uint64, <-chan Event, context.CancelFunc) {
	return b.SubscribeWithIDs(ctx, userID, userID)
}

// SubscribeWithIDs registers a client keyed by both sessionID and userID.
func (b *Broker) SubscribeWithIDs(ctx context.Context, sessionID, userID string) (uint64, <-chan Event, context.CancelFunc) {
	id := b.nextID.Add(1)
	return b.subscribeWithID(ctx, sessionID, userID, id)
}

// SubscribeWithConnectionID registers a client with an externally allocated
// connection identifier. This is used by the SSE handler when a single client
// subscribes to multiple event streams, so every broker shares the same
// connection ID and page context can be updated on the right connection.
func (b *Broker) SubscribeWithConnectionID(ctx context.Context, sessionID, userID string, id uint64) (<-chan Event, context.CancelFunc) {
	_, ch, cancel := b.subscribeWithID(ctx, sessionID, userID, id)
	// Ensure the broker's own ID generator does not reuse this id later.
	for {
		cur := b.nextID.Load()
		if cur >= id {
			break
		}
		if b.nextID.CompareAndSwap(cur, id) {
			break
		}
	}
	return ch, cancel
}

func (b *Broker) subscribeWithID(ctx context.Context, sessionID, userID string, id uint64) (uint64, <-chan Event, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	c := &Connection{
		id:        id,
		sessionID: sessionID,
		userID:    userID,
		ch:        make(chan Event, b.buf),
		cancel:    cancel,
	}

	b.mu.Lock()
	b.connections[id] = c
	if b.sessions[sessionID] == nil {
		b.sessions[sessionID] = make(map[uint64]bool)
	}
	b.sessions[sessionID][id] = true
	if b.users[userID] == nil {
		b.users[userID] = make(map[string]bool)
	}
	b.users[userID][sessionID] = true
	b.mu.Unlock()

	return id, c.ch, func() {
		cancel()
		b.removeConnection(id)
	}
}

// UpdateConnectionSessionID re-associates a single connection from its old
// sessionID to a new sessionID. Used when the auth layer rotates the session
// token while the SSE connection (opened with the old token) is still active.
func (b *Broker) UpdateConnectionSessionID(connID uint64, newSessionID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	c, ok := b.connections[connID]
	if !ok {
		return false
	}

	oldSessionID := c.sessionID
	if oldSessionID == newSessionID {
		return true
	}

	// Remove from old session index.
	if oldSessionID != "" {
		delete(b.sessions[oldSessionID], connID)
		if len(b.sessions[oldSessionID]) == 0 {
			delete(b.sessions, oldSessionID)
		}
	}

	// Update connection and add to new session index.
	c.sessionID = newSessionID
	if b.sessions[newSessionID] == nil {
		b.sessions[newSessionID] = make(map[uint64]bool)
	}
	b.sessions[newSessionID][connID] = true

	// Keep the user index in sync: the userID is unchanged, but the set of
	// sessions for that user must reflect the new sessionID.
	if c.userID != "" {
		if b.users[c.userID] == nil {
			b.users[c.userID] = make(map[string]bool)
		}
		b.users[c.userID][newSessionID] = true
	}

	return true
}

// UpdateConnectionPage updates the page context for a single connection.
func (b *Broker) UpdateConnectionPage(connID uint64, page string, selectors, fragmentIDs map[string]bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	c, ok := b.connections[connID]
	if !ok {
		return false
	}
	c.page = page
	c.selectors = selectors
	c.fragmentIDs = fragmentIDs
	return true
}

// UpdateSessionPage updates the page context for every connection of a session.
func (b *Broker) UpdateSessionPage(sessionID, page string, selectors, fragmentIDs map[string]bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	ids, ok := b.sessions[sessionID]
	if !ok {
		return false
	}
	for id := range ids {
		if c, ok := b.connections[id]; ok {
			c.page = page
			c.selectors = selectors
			c.fragmentIDs = fragmentIDs
		}
	}
	return true
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

func (b *Broker) PublishToConnection(connectionID uint64, e Event) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	connection, ok := b.connections[connectionID]
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

func (b *Broker) PublishToConnections(connectionIDs []uint64, e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, id := range connectionIDs {
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

// PublishToUser fans an event out to all connections for the given user.
func (b *Broker) PublishToUser(userID string, e Event) {
	b.PublishToUserWithOptions(userID, e, nil)
}

// PublishToUserWithOptions fans an event out to the given user's connections,
// optionally restricted to active sessions and to connections matching Filter.
func (b *Broker) PublishToUserWithOptions(userID string, e Event, opts *PublishOptions) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	sessions := b.users[userID]

	var active map[string]bool
	var filter func(*Connection) bool
	if opts != nil {
		active = opts.ActiveSessionIDs
		filter = opts.Filter
	}

	for sessionID := range sessions {
		if active != nil && !active[sessionID] {
			continue
		}
		for connID := range b.sessions[sessionID] {
			c, ok := b.connections[connID]
			if !ok {
				continue
			}
			if filter != nil && !filter(c) {
				continue
			}
			select {
			case c.ch <- e:
			default:
			}
		}
	}
}

func (b *Broker) PublishToUsers(userIDs []string, e Event) {
	b.PublishToUsersWithOptions(userIDs, e, nil)
}

// PublishToUsersWithOptions fans an event out to multiple users.
func (b *Broker) PublishToUsersWithOptions(userIDs []string, e Event, opts *PublishOptions) {
	userSet := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		userSet[userID] = struct{}{}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	var active map[string]bool
	var filter func(*Connection) bool
	if opts != nil {
		active = opts.ActiveSessionIDs
		filter = opts.Filter
	}

	for userID, userSessions := range b.users {
		if _, ok := userSet[userID]; !ok {
			continue
		}
		for sessionID := range userSessions {
			if active != nil && !active[sessionID] {
				continue
			}
			for connID := range b.sessions[sessionID] {
				c, ok := b.connections[connID]
				if !ok {
					continue
				}
				if filter != nil && !filter(c) {
					continue
				}
				select {
				case c.ch <- e:
				default:
				}
			}
		}
	}
}

// UpdateUserID re-associates all connections matching oldUserID to newUserID.
// Used when a session token rotates but the underlying browser connection stays open.
func (b *Broker) UpdateUserID(oldUserID string, newUserID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, c := range b.connections {
		if c.userID == oldUserID {
			c.userID = newUserID
		}
	}
	if sessions, ok := b.users[oldUserID]; ok {
		if b.users[newUserID] == nil {
			b.users[newUserID] = make(map[string]bool)
		}
		for sessionID := range sessions {
			b.users[newUserID][sessionID] = true
		}
		delete(b.users, oldUserID)
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

	// Only remove the session from the user index when this is the last
	// connection for that session. Other tabs may share the same session ID
	// and must remain routable.
	sessionConns := b.sessions[conn.sessionID]
	if len(sessionConns) <= 1 {
		delete(b.users[conn.userID], conn.sessionID)
		if len(b.users[conn.userID]) == 0 {
			delete(b.users, conn.userID)
		}
	}
	delete(sessionConns, id)
	if len(sessionConns) == 0 {
		delete(b.sessions, conn.sessionID)
	}

	close(conn.ch)
}
