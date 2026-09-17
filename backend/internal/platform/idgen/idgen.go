// Package idgen generates opaque, prefixed identifiers and content digests.
package idgen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// New returns a prefixed random identifier, e.g. "mission_3f9a...".
// 128 bits of randomness make collisions and enumeration infeasible.
func New(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

// Digest returns a self-describing SHA-256 content digest.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}
