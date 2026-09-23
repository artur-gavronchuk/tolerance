package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// APIKeyPrefix distinguishes agent keys from OIDC JWTs in the same
// Authorization header. A JWT never starts with "ak_".
const APIKeyPrefix = "ak_"

// GenerateAPIKey returns a new key (shown to the user exactly once), its
// SHA-256 hex hash (the only thing stored), and a 12-character prefix for
// listing keys without revealing them.
func GenerateAPIKey() (key, hash, prefix string, err error) {
	var b [20]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", "", err
	}
	key = APIKeyPrefix + hex.EncodeToString(b[:])
	return key, HashAPIKey(key), key[:12], nil
}

func HashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func IsAPIKey(token string) bool { return strings.HasPrefix(token, APIKeyPrefix) }
