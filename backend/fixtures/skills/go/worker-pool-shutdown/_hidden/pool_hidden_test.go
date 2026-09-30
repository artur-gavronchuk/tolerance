package pool

import (
	"context"
	"runtime"
	"sort"
	"testing"
	"time"
)

func leakCheck(t *testing.T, workers int, jobs []int) {
	t.Helper()
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	block := make(chan struct{})
	done := make(chan struct{})
	go func() {
		_, _ = Run(ctx, workers, jobs, func(x int) int { <-block; return x })
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

func TestHidden_NoLeakWhenWorkersBlocked(t *testing.T) { leakCheck(t, 4, []int{1, 2, 3, 4, 5, 6}) }

func TestHidden_NoLeakWhenProducerBlocked(t *testing.T) {
	jobs := make([]int, 100)
	leakCheck(t, 1, jobs)
}

func TestHidden_MoreWorkersThanJobs(t *testing.T) {
	got, err := Run(context.Background(), 10, []int{1, 2}, func(x int) int { return x })
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
}

func TestHidden_ManyJobsExactlyOnce(t *testing.T) {
	jobs := make([]int, 500)
	for i := range jobs {
		jobs[i] = i
	}
	got, err := Run(context.Background(), 7, jobs, func(x int) int { return x })
	if err != nil {
		t.Fatal(err)
	}
	sort.Ints(got)
	for i, v := range got {
		if v != i {
			t.Fatalf("result %d missing or duplicated (got %d)", i, v)
		}
	}
}
