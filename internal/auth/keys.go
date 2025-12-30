package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HashKey returns a SHA-256 hash of the key.
func HashKey(key string) string {
	key = strings.TrimSpace(key)

	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}
