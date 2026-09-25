package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/metrics"
)

// newMetricsServer registers this process's metrics and, if scale.metricsAddr
// is set, builds (but does not start) the dedicated listener that serves
// them — never the public mux. It returns nil when metrics are disabled.
//
// Registration always happens, even with metrics disabled, so a later
// config change does not need code changes; it is cheap and every role
// does it exactly once at startup.
func newMetricsServer(scale scaleConfig, pool *db.Pool, log *slog.Logger) *http.Server {
	metrics.SetBuildInfo(scale.role, scale.track, scale.version)
	metrics.RegisterPoolStats(pool.Raw())
	// DB-derived dashboard gauges (arena_proofs, arena_jobs_*, arena_agents_*,
	// arena_users_total) only for roles serving the owner API: every replica
	// of those sees the same cluster-wide numbers, so Grafana is expected to
	// use max() over instances rather than sum(). A worker role does not
	// register this collector at all, so its replicas don't multiply it.
	if scale.role == "api" || scale.role == "all" {
		prometheus.MustRegister(newDBCollector(pool, log))
	}
	if scale.metricsAddr == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return &http.Server{
		Addr:              scale.metricsAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      40 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
}
