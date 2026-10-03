package ttlcache

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func newTest(capacity int64, def time.Duration) (*Cache, *FakeClock) {
	clk := NewFakeClock(t0)
	return New(Config{Capacity: capacity, DefaultTTL: def, Clock: clk}), clk
}

func TestSetGet(t *testing.T) {
	c, _ := newTest(10, 0)
	c.Set("a", 1, 1, 0)
	v, ok := c.Get("a")
	if !ok || v != 1 {
		t.Fatalf("Get(a) = %v, %v", v, ok)
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("Get(b) should miss")
	}
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c, _ := newTest(3, 0)
	c.Set("a", 1, 1, 0)
	c.Set("b", 2, 1, 0)
	c.Set("c", 3, 1, 0)
	c.Get("a")
	c.Set("d", 4, 1, 0)
	if _, ok := c.Get("b"); ok {
		t.Fatal("b should have been evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a should still be cached")
	}
}

func TestTTLExpires(t *testing.T) {
	c, clk := newTest(10, 0)
	c.Set("a", 1, 1, time.Minute)
	clk.Advance(30 * time.Second)
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a should be live after 30s")
	}
	clk.Advance(2 * time.Minute)
	if _, ok := c.Get("a"); ok {
		t.Fatal("a should have expired")
	}
}

func TestDeleteAndCost(t *testing.T) {
	c, _ := newTest(10, 0)
	c.Set("a", 1, 4, 0)
	c.Set("b", 2, 3, 0)
	if c.Cost() != 7 || c.Len() != 2 {
		t.Fatalf("cost=%d len=%d", c.Cost(), c.Len())
	}
	if !c.Delete("a") || c.Delete("a") {
		t.Fatal("Delete should report presence once")
	}
	if c.Cost() != 3 {
		t.Fatalf("cost=%d after delete", c.Cost())
	}
}

func TestOnRemoveCapacity(t *testing.T) {
	var got []string
	clk := NewFakeClock(t0)
	c := New(Config{Capacity: 2, Clock: clk, OnRemove: func(k string, v any, r Reason) {
		got = append(got, k+":"+r.String())
	}})
	c.Set("a", 1, 1, 0)
	c.Set("b", 2, 1, 0)
	c.Set("c", 3, 1, 0)
	if len(got) != 1 || got[0] != "a:evicted" {
		t.Fatalf("events = %v", got)
	}
}
