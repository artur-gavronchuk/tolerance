package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHidden_BackoffSequence(t *testing.T) {
	for i, want := range []time.Duration{100e6, 200e6, 400e6, 800e6, 1600e6, 3200e6} {
		if got := Backoff(i + 1); got != want {
			t.Fatalf("Backoff(%d) = %v, want %v", i+1, got, want)
		}
	}
}

func TestHidden_BackoffCapsAtMax(t *testing.T) {
	for _, n := range []int{10, 20, 40, 1000} {
		if got := Backoff(n); got != Max {
			t.Fatalf("Backoff(%d) = %v, want cap %v", n, got, Max)
		}
	}
}

func TestHidden_BackoffZeroAndNegative(t *testing.T) {
	if Backoff(0) != 0 || Backoff(-3) != 0 {
		t.Fatalf("non-positive attempts must return 0")
	}
}

func TestHidden_DoStopsAtMaxAttempts(t *testing.T) {
	fast(t)
	calls := 0
	boom := errors.New("boom")
	err := Do(context.Background(), 4, func() error { calls++; return boom })
	if !errors.Is(err, boom) || calls != 4 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestHidden_DoReturnsContextErrorWhileWaiting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Do(ctx, 3, func() error {
		calls++
		cancel()
		return errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}
