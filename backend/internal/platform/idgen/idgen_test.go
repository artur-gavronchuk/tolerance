package idgen

import (
	"strings"
	"testing"
)

func TestNewIsPrefixedAndUnique(t *testing.T) {
	a := New("mission")
	b := New("mission")
	if !strings.HasPrefix(a, "mission_") {
		t.Fatalf("expected prefix mission_, got %s", a)
	}
	if a == b {
		t.Fatalf("expected two calls to produce different ids")
	}
}

func TestDigestIsDeterministicAndSelfDescribing(t *testing.T) {
	a := Digest([]byte("hello"))
	b := Digest([]byte("hello"))
	if a != b {
		t.Fatalf("expected same input to produce same digest, got %s and %s", a, b)
	}
	if !strings.HasPrefix(a, "sha256:") {
		t.Fatalf("expected sha256: prefix, got %s", a)
	}
	if Digest([]byte("other")) == a {
		t.Fatalf("expected different input to produce different digest")
	}
}
