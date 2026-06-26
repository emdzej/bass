package pairing

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/emdzej/bass/internal/apps"
	"github.com/emdzej/bass/internal/auth"
	"github.com/emdzej/bass/internal/devices"
	"github.com/emdzej/bass/internal/httpx"
	"golang.org/x/oauth2"
)

// API wires the /v1/pair/* endpoints.
type API struct {
	Apps         *apps.Store
	Devices      *devices.Store
	Verifier     *auth.Verifier
	Cache        *Cache
	ClientID     string
	ClientSecret string
	// CallbackURL is the absolute URL clients are sent back to after the IdP
	// authorize step — i.e. this service's own /v1/pair/callback endpoint.
	CallbackURL string
}

func (a *API) Register(mux *http.ServeMux) {
	mux.Handle("GET /v1/pair/start", http.HandlerFunc(a.start))
	mux.Handle("GET /v1/pair/callback", http.HandlerFunc(a.callback))
}

func (a *API) start(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	appID := q.Get("app_id")
	redirectURI := q.Get("redirect_uri")
	deviceLabel := q.Get("device_label")
	mode := q.Get("mode")
	if mode == "" {
		mode = "redirect"
	}
	if appID == "" || redirectURI == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_request", "app_id and redirect_uri required")
		return
	}
	app, err := a.Apps.Get(r.Context(), appID)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "unknown_app", err.Error())
		return
	}
	if !app.AllowsRedirectURI(redirectURI) {
		httpx.Error(w, http.StatusForbidden, "invalid_redirect_uri",
			"redirect_uri not registered for this app")
		return
	}

	// Auth disabled (dev) — short-circuit by minting a device for a synthetic
	// "anonymous" user and redirecting straight back. Useful for client-lib
	// dev without a running IdP.
	if a.Verifier == nil {
		p, err := a.Devices.Create(r.Context(), "dev-user", appID, deviceLabel)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		http.Redirect(w, r, buildCallbackRedirect(redirectURI, p, int(a.Devices.TokenTTL.Seconds())), http.StatusFound)
		return
	}

	verifier, err := NewPKCEVerifier()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	nonce, err := NewNonce()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	stateTok, err := a.Cache.Put(State{
		AppID:        appID,
		RedirectURI:  redirectURI,
		DeviceLabel:  deviceLabel,
		Mode:         mode,
		PKCEVerifier: verifier,
		Nonce:        nonce,
	})
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	cfg := a.Verifier.OAuth2Config(a.ClientID, a.ClientSecret, a.CallbackURL,
		[]string{"openid", "profile", "email", auth.ScopeSync})
	authURL := cfg.AuthCodeURL(stateTok,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", PKCEChallenge(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("nonce", nonce),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (a *API) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	stateTok := q.Get("state")
	code := q.Get("code")
	if errStr := q.Get("error"); errStr != "" {
		httpx.Error(w, http.StatusBadRequest, "idp_error", errStr+": "+q.Get("error_description"))
		return
	}
	if stateTok == "" || code == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid_request", "state and code required")
		return
	}
	st, err := a.Cache.Take(stateTok)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_state", err.Error())
		return
	}

	cfg := a.Verifier.OAuth2Config(a.ClientID, a.ClientSecret, a.CallbackURL,
		[]string{"openid", "profile", "email", auth.ScopeSync})
	tok, err := cfg.Exchange(r.Context(), code,
		oauth2.SetAuthURLParam("code_verifier", st.PKCEVerifier))
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "token_exchange", err.Error())
		return
	}
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		httpx.Error(w, http.StatusBadGateway, "missing_id_token", "IdP did not return id_token")
		return
	}
	claims, err := a.Verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "invalid_id_token", err.Error())
		return
	}
	// Optional but recommended: require bass.sync scope on the user's token.
	if !claims.HasScope(auth.ScopeSync) {
		httpx.Error(w, http.StatusForbidden, "missing_scope",
			"user token must include scope "+auth.ScopeSync)
		return
	}
	if userNonce, _ := claims.Raw["nonce"].(string); userNonce != "" && userNonce != st.Nonce {
		httpx.Error(w, http.StatusBadRequest, "nonce_mismatch", "OIDC nonce mismatch")
		return
	}

	p, err := a.Devices.Create(r.Context(), claims.Subject, st.AppID, st.DeviceLabel)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	http.Redirect(w, r,
		buildCallbackRedirect(st.RedirectURI, p, int(a.Devices.TokenTTL.Seconds())),
		http.StatusFound,
	)
}

// buildCallbackRedirect appends tokens to the redirect URI's fragment.
// Fragment (not query) so tokens are not sent to the host app's web server
// and don't appear in access logs.
func buildCallbackRedirect(redirectURI string, p *devices.Pairing, expiresIn int) string {
	frag := url.Values{}
	frag.Set("sync_token", p.SyncToken)
	frag.Set("refresh_token", p.RefreshToken)
	frag.Set("device_id", p.Device.ID)
	frag.Set("expires_in", strconv.Itoa(expiresIn))
	return redirectURI + "#" + frag.Encode()
}

// RunSweeper periodically clears expired state entries from the cache.
func (a *API) RunSweeper(ctx context.Context, interval int) {
	if interval <= 0 {
		interval = 60
	}
	t := time.NewTicker(time.Duration(interval) * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.Cache.Sweep()
		}
	}
}
