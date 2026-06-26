package sync

import (
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/emdzej/bass/internal/apps"
	"github.com/emdzej/bass/internal/devices"
	"github.com/emdzej/bass/internal/httpx"
)

// Publisher is implemented by changes.Hub — injected so we can fan out
// change notifications without importing the changes package here (which
// would risk a cycle if we ever do).
type Publisher interface {
	Publish(userSub, appID string, cursor int64)
}

// API wires GET/POST /v1/sync.
type API struct {
	Store         *Store
	Apps          *apps.Store
	Devices       *devices.Store
	Publisher     Publisher
	MaxValueBytes int
	MaxBatchItems int
}

func (a *API) Register(mux *http.ServeMux) {
	mux.Handle("GET /v1/sync", a.Devices.Middleware(http.HandlerFunc(a.pull)))
	mux.Handle("POST /v1/sync", a.Devices.Middleware(http.HandlerFunc(a.push)))
}

func (a *API) pull(w http.ResponseWriter, r *http.Request) {
	d, _ := devices.FromContext(r.Context())
	q := r.URL.Query()
	since, _ := strconv.ParseInt(q.Get("since"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > a.MaxBatchItems {
		limit = a.MaxBatchItems
	}
	items, maxVer, err := a.Store.Pull(r.Context(), d.UserSub, d.AppID, since, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	cursor := maxVer
	if len(items) == 0 {
		// Even with no items, return the current counter so the client's
		// cursor advances and avoids re-fetching the same empty window.
		c, _ := a.Store.CurrentVersion(r.Context(), d.UserSub, d.AppID)
		if c > cursor {
			cursor = c
		}
	}
	encoded := make([]Item, len(items))
	for i, it := range items {
		it.ValueB64 = base64.StdEncoding.EncodeToString(it.Value)
		it.Value = nil
		encoded[i] = it
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items":    encoded,
		"cursor":   cursor,
		"has_more": len(items) == limit,
	})
}

type pushBody struct {
	Items []pushItem `json:"items"`
}

type pushItem struct {
	Key         string `json:"key"`
	Value       string `json:"value,omitempty"` // base64
	PayloadVer  int    `json:"payload_ver,omitempty"`
	BaseVersion int64  `json:"base_version,omitempty"`
	Deleted     bool   `json:"deleted,omitempty"`
}

func (a *API) push(w http.ResponseWriter, r *http.Request) {
	d, _ := devices.FromContext(r.Context())
	var body pushBody
	if !httpx.DecodeJSON(w, r, &body) {
		return
	}
	if len(body.Items) == 0 {
		httpx.Error(w, http.StatusBadRequest, "invalid_request", "items required")
		return
	}
	if len(body.Items) > a.MaxBatchItems {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "batch_too_large",
			"max items per batch: "+strconv.Itoa(a.MaxBatchItems))
		return
	}
	app, err := a.Apps.Get(r.Context(), d.AppID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	intents := make([]WriteIntent, 0, len(body.Items))
	for _, it := range body.Items {
		if it.Key == "" {
			httpx.Error(w, http.StatusBadRequest, "invalid_request", "key required")
			return
		}
		if !app.AllowsKey(it.Key) {
			httpx.Error(w, http.StatusForbidden, "forbidden_key",
				"key not permitted by app allowlist: "+it.Key)
			return
		}
		var raw []byte
		if !it.Deleted && it.Value != "" {
			decoded, err := base64.StdEncoding.DecodeString(it.Value)
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "invalid_value",
					"value must be base64 for key "+it.Key)
				return
			}
			raw = decoded
		}
		if len(raw) > a.MaxValueBytes {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "value_too_large",
				"value exceeds max bytes for key "+it.Key)
			return
		}
		intents = append(intents, WriteIntent{
			Key:         it.Key,
			Value:       raw,
			PayloadVer:  it.PayloadVer,
			BaseVersion: it.BaseVersion,
			Deleted:     it.Deleted,
		})
	}

	results, cursor, err := a.Store.Apply(r.Context(), d.UserSub, d.AppID, d.ID, intents)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if a.Publisher != nil {
		a.Publisher.Publish(d.UserSub, d.AppID, cursor)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"results": results,
		"cursor":  cursor,
	})
}
