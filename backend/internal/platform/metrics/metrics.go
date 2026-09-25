// Package metrics is the single place every module's Prometheus
// instrumentation lives, so metric names (which another team's Grafana
// dashboards are built against) are declared once instead of scattered
// across packages with room for typos. Everything here registers into the
// default Prometheus registry; cmd/api serves it on its own listener.
package metrics

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ---- HTTP ----

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "arena_http_requests_total",
		Help: "Total HTTP requests, by method, matched route and status code.",
	}, []string{"method", "route", "code"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "arena_http_request_duration_seconds",
		Help: "HTTP request duration in seconds, by method and matched route.",
		// The connector's long-poll route legitimately takes up to ~25s.
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15, 20, 25, 30},
	}, []string{"method", "route"})
)

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status, s.wroteHeader = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

// HTTPMiddleware records arena_http_requests_total and
// arena_http_request_duration_seconds for every request that reaches it.
// It must wrap the routing mux directly, with no other middleware in
// between that calls r.WithContext (that would hand the mux a different
// *http.Request value than the one HTTPMiddleware holds, so the mux's
// mutation of r.Pattern would not be visible here).
//
// The route label is r.Pattern read after next has served the request:
// with the Go 1.22+ ServeMux, every nested mux the request passes through
// overwrites r.Pattern with its own matched pattern as routing descends,
// so by the time next.ServeHTTP returns, r.Pattern holds the most specific
// pattern matched anywhere in the chain ("" when nothing matched, reported
// as "unmatched"). Raw request paths are never used as a label: with path
// parameters or probing traffic that is unbounded cardinality.
//
// If WithRouteHolder was used further out in the chain (by an outer
// request-logging middleware that itself needs r.WithContext, and so
// cannot read r.Pattern directly for the same reason), the resolved route
// is also recorded there so that outer middleware can retrieve it via
// RouteFromContext.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		if h, ok := r.Context().Value(routeHolderKey{}).(*routeHolder); ok {
			h.route = route
		}
		httpRequests.WithLabelValues(r.Method, route, strconv.Itoa(sr.status)).Inc()
		httpDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

type routeHolder struct{ route string }

type routeHolderKey struct{}

// WithRouteHolder attaches an empty holder that HTTPMiddleware fills in
// with the resolved route once the request has actually been routed.
//
// This exists for the same reason identity.ActorLog does: an outer
// request-logging middleware typically needs its own r.WithContext call
// (to attach a request id, say), which produces a new *http.Request value.
// r.Pattern set deeper in the chain lives on a *different* Request value
// and so is invisible to that outer middleware afterwards, while a context
// value — this holder — is not: every rebind wraps rather than replaces
// the existing context, so a pointer stored in it is shared all the way
// up, regardless of how many such rebinds happen in between.
func WithRouteHolder(ctx context.Context) context.Context {
	return context.WithValue(ctx, routeHolderKey{}, &routeHolder{})
}

// RouteFromContext returns the route HTTPMiddleware resolved for this
// request, or "" if none was recorded (WithRouteHolder was never called
// for this request, or HTTPMiddleware has not run yet).
func RouteFromContext(ctx context.Context) string {
	h, _ := ctx.Value(routeHolderKey{}).(*routeHolder)
	if h == nil {
		return ""
	}
	return h.route
}

// ---- proof worker / sandbox ----

var (
	SandboxRunSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "arena_sandbox_run_seconds",
		Help:    "Duration of a sandbox run in seconds.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 15, 30, 60, 90, 120, 180, 240, 300},
	})
	ProofVerdicts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "arena_proof_verdicts_total",
		Help: "Proof verdicts recorded by the worker, by status (passed, failed, infra_error).",
	}, []string{"status"})
	WorkersBusy = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "arena_workers_busy",
		Help: "Number of proof worker loops in this process currently running a job.",
	})
	WorkerConcurrency = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "arena_worker_concurrency",
		Help: "Configured number of proof worker loops in this process.",
	})
)

// ---- games (tanks matches) ----

var (
	MatchRunSeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "arena_match_run_seconds",
		Help:    "Duration of a tanks match run (match.Run) in seconds.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 15, 30, 60, 90, 120, 180},
	})
	matchesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "arena_matches_total",
		Help: "Tanks matches finished, by result (finished, infra_error).",
	}, []string{"result"})
)

// MatchFinished records a match reaching a terminal state (mirrors ProofVerdicts below for proofs).
func MatchFinished(result string) { matchesTotal.WithLabelValues(result).Inc() }

// MatchesFinishedAdd records n matches reaching a terminal state at once, for a bulk sweep (SweepStuck)
// where issuing one UPDATE already covers every match instead of finishing them one at a time.
func MatchesFinishedAdd(result string, n int) { matchesTotal.WithLabelValues(result).Add(float64(n)) }

// ---- rate limiting ----

var rateLimited = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "arena_rate_limited_total",
	Help: "Requests rejected by rate limiting, by scope (ip, key, signup, login, ...).",
}, []string{"scope"})

func RateLimited(scope string) { rateLimited.WithLabelValues(scope).Inc() }

// ---- oauth ----

var oauthLogins = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "arena_oauth_logins_total",
	Help: "OAuth sign-in attempts completed at the callback, by provider and result (ok, denied, state, failed, email_unverified, rate_limited).",
}, []string{"provider", "result"})

// OAuthLogin records one OAuth callback outcome.
func OAuthLogin(provider, result string) { oauthLogins.WithLabelValues(provider, result).Inc() }

// ---- build info ----

var buildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
	Name: "arena_build_info",
	Help: "Always 1; role identifies what this process runs, track distinguishes a canary release from stable, version is the deployed image tag.",
}, []string{"role", "track", "version"})

// SetBuildInfo records this process's role, release track ("stable" or
// "canary", from ARENA_TRACK) and version (the deployed image tag, from
// ARENA_VERSION). Call once at startup.
func SetBuildInfo(role, track, version string) {
	buildInfo.WithLabelValues(role, track, version).Set(1)
}

// ---- pgxpool stats ----

// RegisterPoolStats exposes the pool's live stats as gauges. Call at most
// once per pool per process; a second call panics on duplicate
// registration, same as any other promauto metric.
func RegisterPoolStats(pool *pgxpool.Pool) {
	gauge := func(name, help string, f func(*pgxpool.Stat) float64) {
		promauto.NewGaugeFunc(prometheus.GaugeOpts{Name: name, Help: help}, func() float64 { return f(pool.Stat()) })
	}
	gauge("arena_db_pool_total_conns", "Total connections currently held by the pool.",
		func(s *pgxpool.Stat) float64 { return float64(s.TotalConns()) })
	gauge("arena_db_pool_acquired_conns", "Connections currently acquired from the pool.",
		func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) })
	gauge("arena_db_pool_idle_conns", "Idle connections currently sitting in the pool.",
		func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) })
	gauge("arena_db_pool_max_conns", "Configured maximum pool size.",
		func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) })
	gauge("arena_db_pool_empty_acquire_total", "Cumulative acquires that had to wait because the pool was empty (counter-like; monotonic).",
		func(s *pgxpool.Stat) float64 { return float64(s.EmptyAcquireCount()) })
}
