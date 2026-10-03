// Package batcher groups items into batches and delivers them to a callback.
package batcher

import (
	"errors"
	"sync"
	"time"
)

// ErrClosed is returned by Add after Close.
var ErrClosed = errors.New("batcher: closed")

// Config configures a Batcher.
type Config struct {
	MaxItems int           // cut a batch at this many items; <= 0: no limit
	MaxDelay time.Duration // cut a batch this long after its first item; <= 0: no limit
	Flush    func(batch []string)
	Clock    Clock // nil means the real clock
}

// Batcher collects items and delivers them in batches.
type Batcher struct {
	mu     sync.Mutex
	cfg    Config
	buf    []string
	timer  Timer
	closed bool
}

// New returns a ready Batcher.
func New(cfg Config) *Batcher {
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}
	return &Batcher{cfg: cfg}
}

// Add appends an item to the pending batch.
func (b *Batcher) Add(item string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return ErrClosed
	}
	b.buf = append(b.buf, item)
	if b.cfg.MaxItems > 0 && len(b.buf) >= b.cfg.MaxItems {
		b.flushLocked()
		return nil
	}
	if b.cfg.MaxDelay > 0 {
		if b.timer != nil {
			b.timer.Stop()
		}
		b.timer = b.cfg.Clock.AfterFunc(b.cfg.MaxDelay, b.onTimer)
	}
	return nil
}

// Close delivers the pending batch and stops the batcher.
func (b *Batcher) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.flushLocked()
}

func (b *Batcher) onTimer() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.flushLocked()
}

func (b *Batcher) flushLocked() {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
	if len(b.buf) == 0 {
		return
	}
	batch := b.buf
	b.buf = b.buf[:0]
	b.cfg.Flush(batch)
}
