// Package keys implements the credential primitives shared by the CLI:
// admin tokens, API keys (generate/hash/verify), always constant-time where
// it matters. Generated applications ship their own independent copy of the
// verify side inside the template — the CLI library is never a runtime
// dependency of generated code.
package keys

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// APIKeyPrefix is the recognizable prefix of every generated app API key.
const APIKeyPrefix = "lbt_live_"

// NewAdminToken returns a fresh 256-bit hex token used to authenticate the
// CLI against the generated app's loopback admin API.
func NewAdminToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate admin token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// NewAPIKey returns (fullKey, prefix). The full key is shown exactly once at
// creation time; only HashKey(full) and prefix are ever stored. The prefix
// is the first 8 characters after the lbt_live_ marker, for display and
// lookup assistance.
func NewAPIKey() (string, string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate api key: %w", err)
	}
	body := base64.RawURLEncoding.EncodeToString(buf) // ~43 chars, base62-ish
	full := APIKeyPrefix + body
	prefix := APIKeyPrefix + body[:8]
	return full, prefix, nil
}

// HashKey returns the hex SHA-256 of a key — this (never the key itself) is
// what gets persisted.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// VerifyKey compares a presented key against a stored hash in constant time.
func VerifyKey(presented, storedHash string) bool {
	got := HashKey(presented)
	return subtle.ConstantTimeCompare([]byte(got), []byte(storedHash)) == 1
}

// KeyPrefix returns the display prefix (first 8 body chars) of a full key.
func KeyPrefix(full string) string {
	_, rest, ok := strings.Cut(full, APIKeyPrefix)
	if !ok || len(rest) < 8 {
		return full
	}
	return APIKeyPrefix + rest[:8]
}
