package identity

import (
	"strings"
	"testing"
)

func TestHashPassword_VerifiesAndRejects(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("unexpected encoding: %s", h)
	}
	if !VerifyPassword(h, "correct horse battery") {
		t.Fatalf("correct password rejected")
	}
	if VerifyPassword(h, "correct horse batter") {
		t.Fatalf("wrong password accepted")
	}
	if VerifyPassword("garbage", "x") {
		t.Fatalf("malformed hash accepted")
	}
	h2, _ := HashPassword("correct horse battery")
	if h2 == h {
		t.Fatalf("salt must differ between hashes")
	}
}
