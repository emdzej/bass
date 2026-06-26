// Package devices manages per-(user, app, device) sync and refresh tokens.
//
// Tokens are opaque random strings — never JWTs. They are stored hashed
// (SHA-256) at rest so a database leak doesn't grant sync access, and
// compared in constant time on the request path.
package devices

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
)

// tokenBytes is the random payload size before base64url encoding (32 bytes
// → 43 chars). Plenty for a bearer token.
const tokenBytes = 32

// MintToken returns a fresh, URL-safe opaque token.
func MintToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the hex-encoded SHA-256 of a token, used as the lookup
// key in the devices table.
func HashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// ConstantTimeEqual compares two hashes without leaking timing.
func ConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// ErrInvalidToken is returned when a token does not match any active device.
var ErrInvalidToken = errors.New("invalid token")
