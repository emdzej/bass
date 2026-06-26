// Package changes implements the WebSocket notification channel.
//
// On every successful sync write, the sync handler calls Hub.Publish — the
// hub then fans out a {"type":"change","cursor":N} JSON message to every
// connection subscribed to that (user_sub, app_id) pair. Clients respond by
// pulling /v1/sync?since=<cursor>.
//
// Payloads never travel over the WS — see SPEC §8.
package changes

import (
	"sync"
)

// Hub is the in-memory connection registry.
type Hub struct {
	mu    sync.Mutex
	conns map[key]map[*Conn]struct{}
}

type key struct {
	UserSub string
	AppID   string
}

// Conn is the server-side handle to a subscribed WebSocket client.
type Conn struct {
	Send func(cursor int64) // non-blocking; hub coalesces by dropping if full
}

func NewHub() *Hub {
	return &Hub{conns: make(map[key]map[*Conn]struct{})}
}

// Register adds a connection for (user_sub, app_id). The returned function
// unregisters it; call it on connection close.
func (h *Hub) Register(userSub, appID string, c *Conn) func() {
	k := key{userSub, appID}
	h.mu.Lock()
	if h.conns[k] == nil {
		h.conns[k] = make(map[*Conn]struct{})
	}
	h.conns[k][c] = struct{}{}
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		if set, ok := h.conns[k]; ok {
			delete(set, c)
			if len(set) == 0 {
				delete(h.conns, k)
			}
		}
		h.mu.Unlock()
	}
}

// Publish wakes up all connections subscribed to (user_sub, app_id).
func (h *Hub) Publish(userSub, appID string, cursor int64) {
	k := key{userSub, appID}
	h.mu.Lock()
	set := h.conns[k]
	// Snapshot the connections so we don't hold the lock while sending.
	conns := make([]*Conn, 0, len(set))
	for c := range set {
		conns = append(conns, c)
	}
	h.mu.Unlock()
	for _, c := range conns {
		c.Send(cursor)
	}
}
