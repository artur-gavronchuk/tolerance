package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func fast(t *testing.T) {
	t.Helper()
	old := after
	after = func(time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		ch <- time.Now()
		return ch
	}
	t.Cleanup(func() { after = old })
}

func TestDo_SucceedsFirstTry(t *testing.T) {
	fast(t)
	calls := 0
	if err := Do(context.Background(), 3, func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestDo_RetriesUntilSuccess(t *testing.T) {
	fast(t)
	calls := 0
	err := Do(context.Background(), 5, func() error {
		calls++
		if calls < 3 {
			return errors.New("not yet")
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestBackoff_Doubles(t *testing.T) {
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	for i, w := range want {
		if got := Backoff(i + 1); got != w {
			t.Fatalf("Backoff(%d) = %v, want %v", i+1, got, w)
		}
	}
}
