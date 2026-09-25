package main

import (
	"net/http"
	"strings"
	"time"

	"tolerance/internal/identity"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/metrics"
	"tolerance/internal/platform/ratelimit"
)

// businessLimits enforces coarse per-owner write ceilings: not an
// authentication-abuse guard (globalRateLimit and identity's own
// login/signup limits already cover that), just a backstop against a
// scripted or stuck caller hammering agent/key/proof creation. In-memory
// per replica is acceptable here — these are generous ceilings meant to
// catch runaway retries, not to be exact under a load balancer.
//
// It wraps the owner mux from inside the session middleware (so
// identity.MustFromContext has a user to key on), not the whole api mux
// from outside.
func businessLimits(limiter *ratelimit.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rule, ok := businessRuleFor(r)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		userID := identity.MustFromContext(r.Context()).UserID
		if !limiter.Allow(rule.scope+":"+userID, rule.limit, rule.window) {
			metrics.RateLimited(rule.scope)
			w.Header().Set("Retry-After", "60")
			httpx.WriteError(w, r, httpx.New(http.StatusTooManyRequests, "rate_limited", "Too many requests; try again later"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

type businessRule struct {
	scope  string
	limit  int
	window time.Duration
}

// businessRuleFor matches on method and exact/prefix path rather than
// r.Pattern: this middleware runs before the owner mux has routed the
// request, so no pattern has been matched yet.
func businessRuleFor(r *http.Request) (businessRule, bool) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent":
		return businessRule{"agent_create", 10, time.Hour}, true
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/agent/keys":
		return businessRule{"key_create", 10, time.Hour}, true
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/proofs":
		return businessRule{"proof_create", 30, time.Hour}, true
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/proofs/") && strings.HasSuffix(r.URL.Path, "/retry"):
		return businessRule{"proof_retry", 30, time.Hour}, true
	default:
		return businessRule{}, false
	}
}
