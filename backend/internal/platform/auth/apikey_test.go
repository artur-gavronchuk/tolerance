package auth_test

import (
	"regexp"
	"testing"

	"tolerance/internal/platform/auth"
)

func TestGenerateAPIKey_ShapeAndHash(t *testing.T) {
	key, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^ak_[0-9a-f]{40}$`).MatchString(key) {
		t.Fatalf("key %q must be ak_ + 40 hex", key)
	}
	if prefix != key[:12] {
		t.Fatalf("prefix must be the first 12 chars, got %q", prefix)
	}
	if hash != auth.HashAPIKey(key) || len(hash) != 64 {
		t.Fatalf("hash must be sha256 hex of the key")
	}
	if !auth.IsAPIKey(key) || auth.IsAPIKey("eyJhbGciOi…") {
		t.Fatal("IsAPIKey must recognise the ak_ prefix only")
	}
	key2, _, _, _ := auth.GenerateAPIKey()
	if key2 == key {
		t.Fatal("keys must be random")
	}
}
