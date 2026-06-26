package apps

import (
	"errors"
	"net/http"

	"github.com/emdzej/bass/internal/auth"
	"github.com/emdzej/bass/internal/httpx"
)

// API exposes the admin REST endpoints for app registration.
type API struct {
	Store    *Store
	Verifier *auth.Verifier
}

// Register wires routes onto mux. All routes require the bass.admin
// scope (or no auth if Verifier is nil, for dev).
func (a *API) Register(mux *http.ServeMux) {
	guard := func(h http.HandlerFunc) http.Handler {
		if a.Verifier == nil {
			return h
		}
		return a.Verifier.Middleware(auth.ScopeAdmin, h)
	}
	mux.Handle("POST /v1/admin/apps", guard(a.create))
	mux.Handle("GET /v1/admin/apps", guard(a.list))
	mux.Handle("GET /v1/admin/apps/{id}", guard(a.get))
	mux.Handle("PATCH /v1/admin/apps/{id}", guard(a.patch))
	mux.Handle("DELETE /v1/admin/apps/{id}", guard(a.delete))
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	var body App
	if !httpx.DecodeJSON(w, r, &body) {
		return
	}
	if body.ID == "" || body.Name == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_request", "id and name are required")
		return
	}
	if err := a.Store.Create(r.Context(), body); err != nil {
		if errors.Is(err, ErrConflict) {
			httpx.Error(w, http.StatusConflict, "already_exists", err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	created, _ := a.Store.Get(r.Context(), body.ID)
	httpx.WriteJSON(w, http.StatusCreated, created)
}

func (a *API) list(w http.ResponseWriter, r *http.Request) {
	apps, err := a.Store.List(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"apps": apps})
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	app, err := a.Store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, app)
}

func (a *API) patch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := a.Store.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	var patch struct {
		Name         *string   `json:"name,omitempty"`
		Origins      *[]string `json:"origins,omitempty"`
		RedirectURIs *[]string `json:"redirect_uris,omitempty"`
		KeyAllowlist *[]string `json:"key_allowlist,omitempty"`
	}
	if !httpx.DecodeJSON(w, r, &patch) {
		return
	}
	if patch.Name != nil {
		existing.Name = *patch.Name
	}
	if patch.Origins != nil {
		existing.Origins = *patch.Origins
	}
	if patch.RedirectURIs != nil {
		existing.RedirectURIs = *patch.RedirectURIs
	}
	if patch.KeyAllowlist != nil {
		existing.KeyAllowlist = *patch.KeyAllowlist
	}
	if err := a.Store.Update(r.Context(), *existing); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	updated, _ := a.Store.Get(r.Context(), id)
	httpx.WriteJSON(w, http.StatusOK, updated)
}

func (a *API) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.Store.Delete(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			w.WriteHeader(http.StatusNoContent) // idempotent
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
