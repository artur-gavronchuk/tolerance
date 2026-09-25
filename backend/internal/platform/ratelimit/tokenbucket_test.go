package ratelimit

import (
	"testing"
	"time"
)

func TestTokenBuckets_AllowsBurstThenLimitsRate(t *testing.T) {
	b := NewTokenBuckets(1, 3, 10)
	for i := 0; i < 3; i++ {
		if !b.Allow("k") {
			t.Fatalf("burst call %d should be allowed", i)
		}
	}
	if b.Allow("k") {
		t.Fatalf("burst exhausted, next call should be limited")
	}
	if !b.Allow("other") {
		t.Fatalf("a different key must have its own bucket")
	}
}

func TestTokenBuckets_EvictsOldestWhenFull(t *testing.T) {
	b := NewTokenBuckets(100, 1, 2)
	b.Allow("a")
	b.Allow("b")
	if b.Len() != 2 {
		t.Fatalf("expected 2 buckets, got %d", b.Len())
	}
	b.Allow("c") // must evict "a", the least recently used
	if b.Len() != 2 {
		t.Fatalf("expected the bucket count to stay capped at 2, got %d", b.Len())
	}
	if _, ok := b.entries["a"]; ok {
		t.Fatalf("expected the oldest key to be evicted")
	}
}

func TestTokenBuckets_EvictIdle(t *testing.T) {
	b := NewTokenBuckets(100, 1, 100)
	b.Allow("a")
	time.Sleep(20 * time.Millisecond)
	b.Allow("b")
	b.EvictIdle(10 * time.Millisecond)
	if _, ok := b.entries["a"]; ok {
		t.Fatalf("idle key must be evicted")
	}
	if _, ok := b.entries["b"]; !ok {
		t.Fatalf("fresh key must survive")
	}
}

func TestConcurrencyLimiter_CapsInFlightPerKey(t *testing.T) {
	c := NewConcurrencyLimiter(2)
	if !c.Acquire("a") || !c.Acquire("a") {
		t.Fatalf("first two acquires must succeed")
	}
	if c.Acquire("a") {
		t.Fatalf("third concurrent acquire must be refused")
	}
	if !c.Acquire("b") {
		t.Fatalf("a different key must have its own budget")
	}
	c.Release("a")
	if !c.Acquire("a") {
		t.Fatalf("after a release, a new acquire must succeed")
	}
}
