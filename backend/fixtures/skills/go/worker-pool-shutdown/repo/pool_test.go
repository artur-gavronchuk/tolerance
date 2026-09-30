package pool

import (
	"context"
	"runtime"
	"sort"
	"testing"
	"time"
)

func TestRun_AllResults(t *testing.T) {
	jobs := []int{1, 2, 3, 4, 5, 6, 7, 8}
	got, err := Run(context.Background(), 3, jobs, func(x int) int { return x * 2 })
	if err != nil {
		t.Fatal(err)
	}
	sort.Ints(got)
	if len(got) != 8 || got[0] != 2 || got[7] != 16 {
		t.Fatalf("got %v", got)
	}
}

func TestRun_CancelledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	block := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, 2, []int{1, 2, 3, 4}, func(x int) int { <-block; return x })
		done <- err
	}()
	cancel()
	err := <-done
	close(block)
	if err != context.Canceled {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestRun_CancelDoesNotLeakGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	block := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _ = Run(ctx, 2, []int{1, 2, 3, 4, 5, 6}, func(x int) int { <-block; return x })
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done
	close(block)
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before {
		t.Fatalf("goroutines leaked: before %d after %d", before, after)
	}
}
