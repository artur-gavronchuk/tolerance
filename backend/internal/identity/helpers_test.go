package identity_test

import (
	"errors"
	"testing"

	"tolerance/internal/platform/httpx"
)

func asProblem(t *testing.T, err error) *httpx.Problem {
	t.Helper()
	var p *httpx.Problem
	if !errors.As(err, &p) {
		t.Fatalf("expected *httpx.Problem, got %v", err)
	}
	return p
}
