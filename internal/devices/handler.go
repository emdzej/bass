package devices

import (
	"net/http"

	"github.com/emdzej/bass/internal/auth"
	"github.com/emdzej/bass/internal/httpx"
)

// API exposes user-facing /v1/devices endpoints and the admin device listing.
type API struct {
	Store    *Store
	Verifier *auth.Verifier
}

func (a *API) Register(mux *http.ServeMux) {
	// User endpoints — authenticated via sync token.
	mux.Handle("GET /v1/devices", a.Store.Middleware(http.HandlerFunc(a.listMine)))
	mux.Handle("DELETE /v1/devices/{id}", a.Store.Middleware(http.HandlerFunc(a.revokeMine)))
	mux.Handle("POST /v1/token/refresh", http.HandlerFunc(a.refresh))

	// Admin endpoint — OIDC scope.
	adminGuard := func(h http.HandlerFunc) http.Handler {
		if a.Verifier == nil {
			return h
		}
		return a.Verifier.Middleware(auth.ScopeAdmin, h)
	}
	mux.Handle("GET /v1/admin/apps/{id}/devices", adminGuard(a.listForApp))
	mux.Handle("DELETE /v1/admin/apps/{id}/devices/{deviceId}", adminGuard(a.revokeAdmin))
}

func (a *API) listMine(w http.ResponseWriter, r *http.Request) {
	d, _ := FromContext(r.Context())
	list, err := a.Store.ListForUserApp(r.Context(), d.UserSub, d.AppID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	type out struct {
		Devices []Device `json:"devices"`
		Current string   `json:"current"`
	}
	httpx.WriteJSON(w, http.StatusOK, out{Devices: list, Current: d.ID})
}

func (a *API) revokeMine(w http.ResponseWriter, r *http.Request) {
	d, _ := FromContext(r.Context())
	id := r.PathValue("id")
	// Ensure the requested device belongs to the same user.
	list, err := a.Store.ListForUserApp(r.Context(), d.UserSub, d.AppID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	owns := false
	for _, x := range list {
		if x.ID == id {
			owns = true
			break
		}
	}
	if !owns {
		httpx.Error(w, http.StatusNotFound, "not_found", "device not found")
		return
	}
	if err := a.Store.Revoke(r.Context(), id); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !httpx.DecodeJSON(w, r, &body) {
		return
	}
	if body.RefreshToken == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_request", "refresh_token required")
		return
	}
	p, err := a.Store.Refresh(r.Context(), body.RefreshToken)
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, "invalid_grant", "refresh token rejected")
		return
	}
	type out struct {
		SyncToken    string `json:"sync_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		DeviceID     string `json:"device_id"`
	}
	httpx.WriteJSON(w, http.StatusOK, out{
		SyncToken:    p.SyncToken,
		RefreshToken: p.RefreshToken,
		ExpiresIn:    int(a.Store.TokenTTL.Seconds()),
		DeviceID:     p.Device.ID,
	})
}

func (a *API) listForApp(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	list, err := a.Store.ListForApp(r.Context(), appID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"devices": list})
}

func (a *API) revokeAdmin(w http.ResponseWriter, r *http.Request) {
	if err := a.Store.Revoke(r.Context(), r.PathValue("deviceId")); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
