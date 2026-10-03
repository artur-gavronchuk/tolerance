// Package limits switches off the per-agent and per-owner quotas (daily
// proof/qualification/upload caps, hourly create caps) for local runs, where
// they only get in the way of trying things. cmd/api calls Disable when
// ARENA_NO_LIMITS=true; every quota check goes through Cap or Disabled.
package limits

import "sync/atomic"

var off atomic.Bool

// Disable turns every quota off for the life of the process.
func Disable() { off.Store(true) }

// Disabled reports whether quotas are off.
func Disabled() bool { return off.Load() }

// Cap returns n, or a ceiling no one reaches when quotas are off.
func Cap(n int) int {
	if off.Load() {
		return 1 << 30
	}
	return n
}
