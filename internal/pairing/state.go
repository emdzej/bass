// Package pairing implements the OIDC code flow that establishes a sync
// token for a (user, app, device) triple.
package pairing

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// State is the per-flow record kept between /v1/pair/start and /v1/pair/callback.
type State struct {
	AppID        string
	RedirectURI  string
	DeviceLabel  string
	Mode         string // "redirect" or "popup"
	PKCEVerifier string
	Nonce        string
	CreatedAt    time.Time
}

// Cache is an in-memory state store with TTL eviction. Capable of carrying
// roughly TTL seconds × steady-state pairing rate. For a single instance.
type Cache struct {
	mu    sync.Mutex
	items map[string]State
	ttl   time.Duration
}

func NewCache(ttl time.Duration) *Cache {
	return &Cache{items: make(map[string]State), ttl: ttl}
}

// Put generates an opaque state token and stores the given record.
func (c *Cache) Put(s State) (string, error) {
	tok, err := randToken(32)
	if err != nil {
		return "", err
	}
	s.CreatedAt = time.Now().UTC()
	c.mu.Lock()
	c.items[tok] = s
	c.mu.Unlock()
	return tok, nil
}

// Take retrieves and deletes the state record for the given token.
// Returns ErrNotFound if missing or expired.
func (c *Cache) Take(tok string) (State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.items[tok]
	if !ok {
		return State{}, ErrNotFound
	}
	delete(c.items, tok)
	if time.Since(s.CreatedAt) > c.ttl {
		return State{}, ErrExpired
	}
	return s, nil
}

// Sweep removes expired entries. Caller runs this on a timer.
func (c *Cache) Sweep() {
	cutoff := time.Now().Add(-c.ttl)
	c.mu.Lock()
	for k, v := range c.items {
		if v.CreatedAt.Before(cutoff) {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}

var (
	ErrNotFound = errors.New("state not found")
	ErrExpired  = errors.New("state expired")
)

func randToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// PKCEChallenge returns the S256-style code_challenge for a verifier.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// NewPKCEVerifier returns a fresh code_verifier (43–128 char allowed; we emit 43).
func NewPKCEVerifier() (string, error) { return randToken(32) }

// NewNonce returns a fresh OIDC nonce.
func NewNonce() (string, error) { return randToken(16) }
