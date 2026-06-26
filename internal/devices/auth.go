package devices

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

type deviceKey struct{}

// FromContext returns the authenticated device attached by Middleware.
func FromContext(ctx context.Context) (*Device, bool) {
	d, ok := ctx.Value(deviceKey{}).(*Device)
	return d, ok
}

// Middleware extracts the Authorization bearer token, looks up the device,
// and attaches it to the request context. Responds 401 on missing/invalid
// tokens.
func (s *Store) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := bearerFromHeader(r.Header.Get("Authorization"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		device, err := s.LookupBySyncToken(r.Context(), token)
		if err != nil {
			http.Error(w, "invalid sync token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), deviceKey{}, device)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerFromHeader(h string) (string, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", errors.New("missing bearer token")
	}
	return strings.TrimSpace(h[len(prefix):]), nil
}
