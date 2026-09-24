package ratelimit

import (
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
