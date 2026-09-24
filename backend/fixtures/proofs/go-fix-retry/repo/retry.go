// Package retry retries an operation with exponential backoff.
package retry

import (
	"context"
	"time"
)

const (
	Base = 100 * time.Millisecond
	Max  = 30 * time.Second
)

// after is a hook so tests can run without real sleeping.
var after = time.After

// Backoff returns the delay before attempt n (1-based).
func Backoff(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	return Base * time.Duration(attempt)
}

// Do calls fn until it succeeds or maxAttempts is used up.
func Do(ctx context.Context, maxAttempts int, fn func() error) error {
	var err error
	for i := 1; i <= maxAttempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-after(Backoff(i)):
		}
	}
	return err
}
