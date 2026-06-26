// Package cors provides a small CORS middleware for bass's HTTP surfaces.
//
// Matching semantics: each AllowedOriginPatterns entry is a glob
// (path.Match) compared against the request's Origin host (lowercased,
// including port). Empty AllowedOriginPatterns disables CORS entirely.
package cors

import (
	"net/http"
	"net/url"
	"path"
	"strings"
)

// Middleware wraps an http.Handler and adds CORS support.
type Middleware struct {
	AllowedOriginPatterns []string
}

// Wrap returns next wrapped with CORS handling. Passthrough if empty.
func (m *Middleware) Wrap(next http.Handler) http.Handler {
	if len(m.AllowedOriginPatterns) == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")

		if origin != "" && m.allowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				if reqHdrs := r.Header.Get("Access-Control-Request-Headers"); reqHdrs != "" {
					w.Header().Set("Access-Control-Allow-Headers", reqHdrs)
				} else {
					w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
				}
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (m *Middleware) allowed(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Host)
	for _, p := range m.AllowedOriginPatterns {
		ok, err := path.Match(strings.ToLower(p), host)
		if err == nil && ok {
			return true
		}
	}
	return false
}
