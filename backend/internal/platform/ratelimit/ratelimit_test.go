package ratelimit

import (
	"fmt"
	"testing"
	"time"
)

func TestLimiter_AllowsUpToLimitThenBlocksUntilWindowPasses(t *testing.T) {
	now := time.Unix(1000, 0)
	l := New(func() time.Time { return now })
	for i := 0; i < 3; i++ {
		if !l.Allow("k", 3, time.Minute) {
			t.Fatalf("call %d should be allowed", i)
		}
	}
	if l.Allow("k", 3, time.Minute) {
		t.Fatalf("4th call should be blocked")
	}
	if !l.Allow("other", 3, time.Minute) {
		t.Fatalf("different key must not be affected")
	}
	now = now.Add(61 * time.Second)
	if !l.Allow("k", 3, time.Minute) {
		t.Fatalf("after the window the key should be allowed again")
	}
}

func TestLimiter_SweepDropsQuietKeysSoMemoryStaysBounded(t *testing.T) {
	now := time.Unix(1000, 0)
	l := New(func() time.Time { return now })

	// Every key here is used exactly once, one minute apart, well before the
	// sweep threshold (sweepEvery calls) is reached; each becomes quiet
	// relative to the 1-minute window as soon as the clock moves on.
	for i := 0; i < sweepEvery+10; i++ {
		l.Allow(fmt.Sprintf("k%d", i), 3, time.Minute)
		now = now.Add(time.Minute)
	}
	if len(l.hits) > 20 {
		t.Fatalf("expected the sweep to have dropped quiet keys, map still has %d entries", len(l.hits))
	}
}
