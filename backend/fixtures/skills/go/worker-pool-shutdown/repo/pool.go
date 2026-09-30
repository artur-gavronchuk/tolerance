// Package pool runs jobs on a fixed number of goroutines.
package pool

import (
	"context"
	"sync"
)

// Run applies fn to every job using `workers` goroutines and returns the results.
func Run(ctx context.Context, workers int, jobs []int, fn func(int) int) ([]int, error) {
	in := make(chan int)
	out := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range in {
				out <- fn(j)
			}
		}()
	}
	go func() {
		for _, j := range jobs {
			in <- j
		}
		close(in)
	}()
	results := make([]int, 0, len(jobs))
	for range jobs {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case r := <-out:
			results = append(results, r)
		}
	}
	wg.Wait()
	return results, nil
}
