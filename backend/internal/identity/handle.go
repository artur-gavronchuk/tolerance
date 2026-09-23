package identity

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"strings"

	"tolerance/internal/platform/auth"
)

var handleRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,31}$`)
var notHandleChar = regexp.MustCompile(`[^a-z0-9]+`)

func ValidHandle(h string) bool { return handleRe.MatchString(h) }

// DeriveHandle proposes a handle from OIDC claims: preferred_username, then
// the local part of the email, then a random user-xxxxxx. The result always
// satisfies ValidHandle; uniqueness is the caller's job (ResolveUser adds a
// numeric suffix on collision).
func DeriveHandle(c auth.Claims) string {
	src := c.PreferredUsername
	if src == "" && c.Email != "" {
		src = strings.SplitN(c.Email, "@", 2)[0]
	}
	h := normalizeHandle(src)
	if h == "" {
		var b [3]byte
		_, _ = rand.Read(b[:])
		return "user-" + hex.EncodeToString(b[:])
	}
	return h
}

func normalizeHandle(s string) string {
	s = strings.ToLower(s)
	s = notHandleChar.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return ""
	}
	if len(s) < 2 {
		s += "-user"
	}
	if len(s) > 32 {
		s = strings.TrimRight(s[:32], "-")
	}
	return s
}
