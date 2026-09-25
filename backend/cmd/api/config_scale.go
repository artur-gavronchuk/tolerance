package main

// This file holds the horizontal-scaling and hardening knobs added on top
// of config.go's original fields, kept separate so it doesn't collide with
// unrelated edits there. Everything here is loaded by loadScaleConfig,
// called alongside loadConfig in main.go.

import (
	"errors"
	"net"
	"os"
	"strconv"
)

// scaleConfig is the deployment-shape and rate-limiting configuration: what
// role this process plays, how many proof workers it runs, where (if
// anywhere) it exposes Prometheus metrics, and the request-rate ceilings
// for the global rate-limiting middleware.
type scaleConfig struct {
	role              string // "all" | "api" | "worker"
	workerConcurrency int
	metricsAddr       string // "" disables the metrics listener
	trustProxy        bool
	track             string // "stable" | "canary", ARENA_TRACK; reported on arena_build_info
	version           string // ARENA_VERSION, the deployed image tag; reported on arena_build_info

	rateIPRPS    float64
	rateIPBurst  int
	rateKeyRPS   float64
	rateKeyBurst int
}

func loadScaleConfig() (scaleConfig, error) {
	sc := scaleConfig{
		role:              env("ARENA_ROLE", "all"),
		workerConcurrency: envInt("ARENA_WORKER_CONCURRENCY", 1),
		metricsAddr:       os.Getenv("ARENA_METRICS_ADDR"),
		trustProxy:        os.Getenv("ARENA_TRUST_PROXY") == "true",
		rateIPRPS:         envFloat("ARENA_RATE_IP_RPS", 20),
		rateIPBurst:       envInt("ARENA_RATE_IP_BURST", 60),
		rateKeyRPS:        envFloat("ARENA_RATE_KEY_RPS", 5),
		rateKeyBurst:      envInt("ARENA_RATE_KEY_BURST", 20),
		track:             env("ARENA_TRACK", "stable"),
		version:           env("ARENA_VERSION", "dev"),
	}
	if sc.role != "all" && sc.role != "api" && sc.role != "worker" {
		return scaleConfig{}, errors.New("ARENA_ROLE must be all, api or worker")
	}
	if sc.track != "stable" && sc.track != "canary" {
		return scaleConfig{}, errors.New("ARENA_TRACK must be stable or canary")
	}
	if sc.workerConcurrency < 1 {
		return scaleConfig{}, errors.New("ARENA_WORKER_CONCURRENCY must be at least 1")
	}
	if sc.metricsAddr != "" {
		host, _, err := net.SplitHostPort(sc.metricsAddr)
		if err != nil || net.ParseIP(host) == nil {
			return scaleConfig{}, errors.New("ARENA_METRICS_ADDR must be an ip:port")
		}
		if !net.ParseIP(host).IsLoopback() && os.Getenv("ARENA_ALLOW_NON_LOOPBACK") != "true" {
			return scaleConfig{}, errors.New("ARENA_METRICS_ADDR must be a loopback address unless ARENA_ALLOW_NON_LOOPBACK=true")
		}
	}
	if sc.rateIPRPS <= 0 || sc.rateIPBurst < 1 || sc.rateKeyRPS <= 0 || sc.rateKeyBurst < 1 {
		return scaleConfig{}, errors.New("ARENA_RATE_* values must be positive")
	}
	return sc, nil
}

func envInt(name string, fallback int) int {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envFloat(name string, fallback float64) float64 {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}
