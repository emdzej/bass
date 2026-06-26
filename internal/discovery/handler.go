// Package discovery serves the public /.well-known/bass-config endpoint.
package discovery

import (
	"net/http"

	"github.com/emdzej/bass/internal/auth"
	"github.com/emdzej/bass/internal/httpx"
)

// Config is the JSON envelope returned to clients.
type Config struct {
	Issuer    string             `json:"issuer,omitempty"`
	Scopes    Scopes             `json:"scopes"`
	Endpoints Endpoints          `json:"endpoints"`
	Limits    Limits             `json:"limits"`
	IdP       *auth.IdPEndpoints `json:"idp_endpoints,omitempty"`
}

type Scopes struct {
	User  string `json:"user"`
	Admin string `json:"admin"`
}

type Endpoints struct {
	PairStart    string `json:"pair_start"`
	PairCallback string `json:"pair_callback"`
	Sync         string `json:"sync"`
	ChangesWS    string `json:"changes_ws"`
	TokenRefresh string `json:"token_refresh"`
	Devices      string `json:"devices"`
}

type Limits struct {
	MaxValueBytes int `json:"max_value_bytes"`
	MaxBatchItems int `json:"max_batch_items"`
}

// Handler returns an http.Handler for GET /.well-known/bass-config.
func Handler(baseURL string, wsBaseURL string, verifier *auth.Verifier, limits Limits) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := Config{
			Scopes: Scopes{
				User:  auth.ScopeSync,
				Admin: auth.ScopeAdmin,
			},
			Endpoints: Endpoints{
				PairStart:    baseURL + "/v1/pair/start",
				PairCallback: baseURL + "/v1/pair/callback",
				Sync:         baseURL + "/v1/sync",
				ChangesWS:    wsBaseURL + "/v1/changes",
				TokenRefresh: baseURL + "/v1/token/refresh",
				Devices:      baseURL + "/v1/devices",
			},
			Limits: limits,
		}
		if verifier != nil {
			cfg.Issuer = verifier.Issuer()
			ep := verifier.Endpoints()
			cfg.IdP = &ep
		}
		httpx.WriteJSON(w, http.StatusOK, cfg)
	})
}
