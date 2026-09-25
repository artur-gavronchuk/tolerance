// Package clientip resolves the real client address behind a reverse
// proxy. It exists as its own small package (rather than living inside
// internal/identity) so every module that authenticates a caller — cookie
// sessions, API keys, and whatever comes next — can key rate limits and
// audit logs on the same address, computed the same way.
package clientip

import (
	"net"
	"net/http"
	"strings"
)

// FromRequest returns the address a rate limiter or audit log should use
// for r.
//
// trustProxy must only be true when every request genuinely passes through
// a reverse proxy that sets X-Real-IP to its own resolved client address
// (this codebase's Caddy does, already accounting for Cloudflare). With
// trustProxy true, that header is used when present and parseable, falling
// back to r.RemoteAddr otherwise. With trustProxy false (the default —
// never trust headers unless a proxy is known to set them), only
// r.RemoteAddr is used, so a client cannot forge the header to share
// another caller's rate-limit bucket or dodge its own.
func FromRequest(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
