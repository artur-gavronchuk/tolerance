package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"tolerance/internal/agents"
	"tolerance/internal/platform/db"
)

// dbCollector computes a handful of cheap, cluster-wide dashboard gauges by
// querying the database at scrape time, rather than keeping them updated
// on every write. It is registered only for roles that serve the owner API
// (api, all): every replica of those roles sees the same underlying
// numbers, so dashboards are expected to use max() over instances rather
// than sum().
//
// It is an "unchecked" collector (Describe sends nothing): the label sets
// for arena_proofs and arena_jobs_pending are only known once the rows come
// back, so there is nothing fixed to describe up front.
type dbCollector struct {
	pool *db.Pool
	log  *slog.Logger
}

func newDBCollector(pool *db.Pool, log *slog.Logger) *dbCollector {
	return &dbCollector{pool: pool, log: log}
}

func (c *dbCollector) Describe(chan<- *prometheus.Desc) {}

var (
	proofsDesc            = prometheus.NewDesc("arena_proofs", "Number of proofs, by status.", []string{"status"}, nil)
	jobsPendingDesc       = prometheus.NewDesc("arena_jobs_pending", "Number of jobs not yet done or failed, by kind.", []string{"kind"}, nil)
	jobsOldestPendingDesc = prometheus.NewDesc("arena_jobs_oldest_pending_age_seconds",
		"Age in seconds of the oldest pending job, by kind.", []string{"kind"}, nil)
	agentsConnectedDesc = prometheus.NewDesc("arena_agents_connected", "Number of agents with fresh presence.", nil, nil)
	usersTotalDesc      = prometheus.NewDesc("arena_users_total", "Number of registered users.", nil, nil)
	agentsTotalDesc     = prometheus.NewDesc("arena_agents_total", "Number of registered agents.", nil, nil)
)

func (c *dbCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw := c.pool.Raw()

	if rows, err := raw.Query(ctx, `SELECT status, count(*) FROM proofs GROUP BY status`); err != nil {
		c.log.Error("metrics: proofs by status", "err", err)
	} else {
		for rows.Next() {
			var status string
			var n float64
			if err := rows.Scan(&status, &n); err != nil {
				c.log.Error("metrics: proofs by status scan", "err", err)
				break
			}
			ch <- prometheus.MustNewConstMetric(proofsDesc, prometheus.GaugeValue, n, status)
		}
		rows.Close()
	}

	if rows, err := raw.Query(ctx, `SELECT kind, count(*) FROM jobs WHERE state NOT IN ('done', 'failed') GROUP BY kind`); err != nil {
		c.log.Error("metrics: jobs pending", "err", err)
	} else {
		for rows.Next() {
			var kind string
			var n float64
			if err := rows.Scan(&kind, &n); err != nil {
				c.log.Error("metrics: jobs pending scan", "err", err)
				break
			}
			ch <- prometheus.MustNewConstMetric(jobsPendingDesc, prometheus.GaugeValue, n, kind)
		}
		rows.Close()
	}

	if rows, err := raw.Query(ctx, `SELECT kind, extract(epoch FROM now() - min(created_at)) FROM jobs
		WHERE state NOT IN ('done', 'failed') GROUP BY kind`); err != nil {
		c.log.Error("metrics: jobs oldest pending", "err", err)
	} else {
		for rows.Next() {
			var kind string
			var age float64
			if err := rows.Scan(&kind, &age); err != nil {
				c.log.Error("metrics: jobs oldest pending scan", "err", err)
				break
			}
			ch <- prometheus.MustNewConstMetric(jobsOldestPendingDesc, prometheus.GaugeValue, age, kind)
		}
		rows.Close()
	}

	var connected float64
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM agent_presence WHERE last_seen_at > now() - make_interval(secs => $1)`,
		agents.PresenceTTL.Seconds()).Scan(&connected); err != nil {
		c.log.Error("metrics: agents connected", "err", err)
	} else {
		ch <- prometheus.MustNewConstMetric(agentsConnectedDesc, prometheus.GaugeValue, connected)
	}

	var users float64
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&users); err != nil {
		c.log.Error("metrics: users total", "err", err)
	} else {
		ch <- prometheus.MustNewConstMetric(usersTotalDesc, prometheus.GaugeValue, users)
	}

	var totalAgents float64
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM agents`).Scan(&totalAgents); err != nil {
		c.log.Error("metrics: agents total", "err", err)
	} else {
		ch <- prometheus.MustNewConstMetric(agentsTotalDesc, prometheus.GaugeValue, totalAgents)
	}
}
