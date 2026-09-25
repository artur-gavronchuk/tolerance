package main

import (
	"net/http"
	"strings"

	"tolerance/internal/platform/auth"
	"tolerance/internal/platform/clientip"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/metrics"
	"tolerance/internal/platform/ratelimit"
)

// globalRateLimit is the outermost abuse guard, applied to every request
// under /api/ (see newHandler; /healthz is mounted outside it and so is
// naturally exempt): a per-IP token bucket, plus, for connector calls
// carrying a Bearer key, a second per-API-key-hash token bucket and a cap
// on concurrent long-polls per key. It runs before routing, so a request
// that would 401 later still consumes a token — that is the point, since
// an unauthenticated flood is exactly what this defends against.
func globalRateLimit(scale scaleConfig, ipLimiter, keyLimiter *ratelimit.TokenBuckets, longPoll *ratelimit.ConcurrencyLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientip.FromRequest(r, scale.trustProxy)
		if !ipLimiter.Allow(ip) {
			metrics.RateLimited("ip")
			tooManyRequests(w, r)
			return
		}
		if keyHash, ok := bearerKeyHash(r); ok {
			if !keyLimiter.Allow(keyHash) {
				metrics.RateLimited("key")
				tooManyRequests(w, r)
				return
			}
			if isLongPollPath(r) {
				if !longPoll.Acquire(keyHash) {
					metrics.RateLimited("long_poll")
					tooManyRequests(w, r)
					return
				}
				defer longPoll.Release(keyHash)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLongPollPath(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/connector/tasks/next")
}

// bearerKeyHash extracts the rate-limit bucket key for a connector call: the
// same SHA-256 hash RequireAgent will look up, computed here without a DB
// round trip since this middleware only needs a stable bucket per key, not
// to validate it (RequireAgent still does that later; an unknown or
// revoked key is still rate limited, which is correct).
func bearerKeyHash(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	tok := strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if tok == "" {
		return "", false
	}
	return auth.HashAPIKey(tok), true
}

func tooManyRequests(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", "1")
	httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many requests"))
}
