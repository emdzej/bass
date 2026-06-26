package changes

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/emdzej/bass/internal/devices"
)

// Handler serves the /v1/changes WebSocket endpoint.
type Handler struct {
	Hub     *Hub
	Devices *devices.Store
	Logger  *slog.Logger
	// AllowedOriginPatterns is passed to coder/websocket.Accept for browser
	// origin validation. Empty = allow all (dev only).
	AllowedOriginPatterns []string
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /v1/changes", http.HandlerFunc(h.serve))
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	// Auth via Sec-WebSocket-Protocol — pattern is two subprotocols:
	// "bass.v1" and "bearer.<token>". coder/websocket Accept will echo
	// back the first subprotocol it knows, so we ask for "bass.v1" only.
	token := extractBearerSubprotocol(r.Header.Get("Sec-WebSocket-Protocol"))
	if token == "" {
		http.Error(w, "missing bearer subprotocol", http.StatusUnauthorized)
		return
	}
	device, err := h.Devices.LookupBySyncToken(r.Context(), token)
	if err != nil {
		http.Error(w, "invalid sync token", http.StatusUnauthorized)
		return
	}

	opts := &websocket.AcceptOptions{
		Subprotocols:    []string{"bass.v1"},
		OriginPatterns:  h.AllowedOriginPatterns,
		CompressionMode: websocket.CompressionDisabled,
	}
	conn, err := websocket.Accept(w, r, opts)
	if err != nil {
		// Accept writes its own response; nothing to do.
		return
	}
	defer func() { _ = conn.CloseNow() }()

	// Send channel — buffered=1 acts as a "wake-up needed" flag. If a write
	// is already pending, new Publish calls noop (we don't care about the
	// exact cursor, only that the client should pull).
	pending := make(chan int64, 1)
	send := func(cursor int64) {
		select {
		case pending <- cursor:
		default:
			// already a wake-up queued
		}
	}
	hubConn := &Conn{Send: send}
	unregister := h.Hub.Register(device.UserSub, device.AppID, hubConn)
	defer unregister()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Read loop — handle subscribe + pong, ignore unknown messages, exit
	// on close/error. We don't strictly need anything from the client after
	// the initial subscribe, but reading is required so coder/websocket can
	// detect closes.
	go func() {
		defer cancel()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				Type  string `json:"type"`
				Since int64  `json:"since,omitempty"`
			}
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "subscribe":
				// No server-side action; client just declared its cursor.
			case "pong":
				// heartbeat
			}
		}
	}()

	// Write loop: heartbeat ping every 30s; wake on Publish.
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case cursor := <-pending:
			payload, _ := json.Marshal(map[string]any{"type": "change", "cursor": cursor})
			wctx, cancelW := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(wctx, websocket.MessageText, payload)
			cancelW()
			if err != nil {
				return
			}
		case t := <-ping.C:
			payload, _ := json.Marshal(map[string]any{"type": "ping", "t": t.Unix()})
			wctx, cancelW := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Write(wctx, websocket.MessageText, payload)
			cancelW()
			if err != nil {
				return
			}
		}
	}
}

// extractBearerSubprotocol pulls the token from a Sec-WebSocket-Protocol
// header value of the form "bass.v1, bearer.<token>". Returns empty if no
// bearer.* entry is found.
func extractBearerSubprotocol(h string) string {
	parts := strings.Split(h, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if rest, ok := strings.CutPrefix(p, "bearer."); ok {
			return rest
		}
	}
	return ""
}
