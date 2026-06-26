// Package auth provides OIDC JWT verification for the bass admin + pairing
// control plane. Data-plane sync tokens are opaque and verified separately
// by the devices package.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Verifier wraps an OIDC ID-token verifier and captures the IdP's authorize
// + token endpoints so the pairing flow can drive the OAuth code exchange.
type Verifier struct {
	verifier  *oidc.IDTokenVerifier
	provider  *oidc.Provider
	audience  string
	issuer    string
	endpoints IdPEndpoints
}

// IdPEndpoints holds the discovery fields needed by clients and the pairing
// handler.
type IdPEndpoints struct {
	AuthorizationEndpoint string `json:"authorization_endpoint,omitempty"`
	TokenEndpoint         string `json:"token_endpoint,omitempty"`
	UserInfoEndpoint      string `json:"userinfo_endpoint,omitempty"`
}

// NewVerifier autodiscovers OIDC config from the issuer URL.
func NewVerifier(ctx context.Context, issuer, audience string) (*Verifier, error) {
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	cfg := &oidc.Config{
		ClientID:          audience,
		SkipClientIDCheck: audience == "",
	}
	ep := provider.Endpoint()
	var raw struct {
		UserInfoEndpoint string `json:"userinfo_endpoint"`
	}
	_ = provider.Claims(&raw)
	return &Verifier{
		verifier: provider.Verifier(cfg),
		provider: provider,
		audience: audience,
		issuer:   issuer,
		endpoints: IdPEndpoints{
			AuthorizationEndpoint: ep.AuthURL,
			TokenEndpoint:         ep.TokenURL,
			UserInfoEndpoint:      raw.UserInfoEndpoint,
		},
	}, nil
}

func (v *Verifier) Issuer() string             { return v.issuer }
func (v *Verifier) Audience() string           { return v.audience }
func (v *Verifier) Endpoints() IdPEndpoints    { return v.endpoints }
func (v *Verifier) Provider() *oidc.Provider   { return v.provider }

// OAuth2Config returns an oauth2.Config suitable for the authorization code
// exchange during pairing. The caller provides the redirect URI and client
// credentials.
func (v *Verifier) OAuth2Config(clientID, clientSecret, redirectURI string, scopes []string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     v.provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       scopes,
	}
}

// Claims captures the bits of a verified ID token we care about.
type Claims struct {
	Subject string
	Email   string
	Name    string
	Scopes  []string
	Raw     map[string]any
}

// Verify validates the bearer token and returns parsed claims.
func (v *Verifier) Verify(ctx context.Context, bearer string) (*Claims, error) {
	tok, err := v.verifier.Verify(ctx, bearer)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := tok.Claims(&raw); err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}
	c := &Claims{Subject: tok.Subject, Raw: raw}
	if s, ok := raw["email"].(string); ok {
		c.Email = s
	}
	if s, ok := raw["name"].(string); ok {
		c.Name = s
	}
	c.Scopes = extractScopes(raw)
	return c, nil
}

func extractScopes(raw map[string]any) []string {
	if v, ok := raw["scope"].(string); ok {
		return strings.Fields(v)
	}
	if arr, ok := raw["scp"].([]any); ok {
		out := make([]string, 0, len(arr))
		for _, s := range arr {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// HasScope reports whether the token includes the given scope.
func (c *Claims) HasScope(s string) bool {
	for _, x := range c.Scopes {
		if x == s {
			return true
		}
	}
	return false
}

// Middleware verifies the Authorization bearer and ensures the listed scope
// is present. The verified Claims are attached to the request context.
func (v *Verifier) Middleware(scope string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, err := bearerFromHeader(r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		claims, err := v.Verify(r.Context(), bearer)
		if err != nil {
			http.Error(w, "invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}
		if scope != "" && !claims.HasScope(scope) {
			http.Error(w, "missing required scope: "+scope, http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey{}, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type claimsKey struct{}

// ClaimsFromContext returns the verified claims attached by Middleware.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*Claims)
	return c, ok
}

func bearerFromHeader(h string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", errors.New("missing bearer token")
	}
	return strings.TrimSpace(h[len(prefix):]), nil
}
