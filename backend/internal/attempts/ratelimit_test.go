package attempts

import (
	"testing"
	"time"
)

func TestLimiter_BurstThenRefill(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newLimiter(4, 4, func() time.Time { return now })
	for i := 0; i < 4; i++ {
		if !l.Allow("a") {
			t.Fatalf("request %d of the burst must pass", i+1)
		}
	}
	if l.Allow("a") {
		t.Fatal("the fifth immediate request must be limited")
	}
	if !l.Allow("b") {
		t.Fatal("keys are independent")
	}
	now = now.Add(250 * time.Millisecond) // one token at 4/s
	if !l.Allow("a") {
		t.Fatal("a token must have refilled after 250ms")
	}
	if l.Allow("a") {
		t.Fatal("only one token refilled")
	}
	now = now.Add(time.Hour)
	for i := 0; i < 4; i++ {
		if !l.Allow("a") {
			t.Fatalf("bucket must refill up to the burst, failed at %d", i+1)
		}
	}
	if l.Allow("a") {
		t.Fatal("refill is capped at the burst")
	}
}
