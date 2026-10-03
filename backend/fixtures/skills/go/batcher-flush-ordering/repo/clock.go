package batcher

import "time"

// Clock is the time source of a Batcher.
type Clock interface {
	Now() time.Time
	// AfterFunc calls f once, on some goroutine, after d has passed. The
	// returned Timer's Stop reports whether it stopped the call before it
	// started; once f has started (or is about to) Stop returns false and f
	// still runs.
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is a pending AfterFunc.
type Timer interface {
	Stop() bool
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, f func()) Timer {
	return time.AfterFunc(d, f)
}
