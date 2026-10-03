package ttlcache

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

var h0 = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

type rec struct {
	key    string
	reason Reason
}

func hidden(capacity int64, def time.Duration, log *[]rec) (*Cache, *FakeClock) {
	clk := NewFakeClock(h0)
	cfg := Config{Capacity: capacity, DefaultTTL: def, Clock: clk}
	if log != nil {
		cfg.OnRemove = func(k string, v any, r Reason) { *log = append(*log, rec{k, r}) }
	}
	return New(cfg), clk
}

func TestHiddenExpiryBoundaryAndAbsoluteTTL(t *testing.T) {
	c, clk := hidden(10, time.Minute, nil)
	c.Set("def", 1, 1, 0)
	c.Set("own", 2, 1, 90*time.Second)
	c.Set("forever", 3, 1, -1)
	clk.Advance(59 * time.Second)
	if _, ok := c.Get("def"); !ok {
		t.Fatal("def live at 59s")
	}
	clk.Advance(500 * time.Millisecond) // 59.5s; a Get must not have extended def's life
	if _, ok := c.Get("def"); !ok {
		t.Fatal("def live at 59.5s")
	}
	clk.Advance(500 * time.Millisecond) // exactly 60s: expired at the boundary instant
	if _, ok := c.Peek("def"); ok {
		t.Fatal("def must be expired exactly at Set+TTL")
	}
	if _, ok := c.Get("def"); ok {
		t.Fatal("def must miss at the boundary")
	}
	clk.Advance(29*time.Second + 999*time.Millisecond)
	if _, ok := c.Get("own"); !ok {
		t.Fatal("own live just before 90s")
	}
	clk.Advance(time.Millisecond)
	if _, ok := c.Get("own"); ok {
		t.Fatal("own expired at 90s")
	}
	clk.Advance(1000 * time.Hour)
	if _, ok := c.Get("forever"); !ok {
		t.Fatal("negative ttl never expires")
	}
	// Set restarts the clock for an entry
	c.Set("x", 1, 1, 10*time.Second)
	clk.Advance(8 * time.Second)
	c.Set("x", 1, 1, 10*time.Second)
	clk.Advance(8 * time.Second)
	if _, ok := c.Get("x"); !ok {
		t.Fatal("re-Set must restart the TTL")
	}
}

func TestHiddenCostAccounting(t *testing.T) {
	c, clk := hidden(10, 0, nil)
	c.Set("a", 1, 4, 0)
	c.Set("a", 2, 6, 0)
	if c.Cost() != 6 || c.Len() != 1 {
		t.Fatalf("after replace cost=%d len=%d, want 6/1", c.Cost(), c.Len())
	}
	c.Set("a", 3, 2, 0)
	c.Set("b", 4, 8, 0)
	if c.Cost() != 10 || c.Len() != 2 {
		t.Fatalf("cost=%d len=%d, want 10/2 (nothing may be evicted)", c.Cost(), c.Len())
	}
	c.Set("c", 5, 0, time.Second) // zero cost is fine and never evicts anything
	if c.Cost() != 10 || c.Len() != 3 {
		t.Fatalf("zero cost: cost=%d len=%d", c.Cost(), c.Len())
	}
	// lazy removal on Get releases the cost
	c.Set("t", 6, 0, time.Second)
	c.Delete("a")
	c.Set("big", 7, 2, time.Second)
	if c.Cost() != 10 {
		t.Fatalf("cost=%d", c.Cost())
	}
	clk.Advance(time.Second)
	if _, ok := c.Get("big"); ok {
		t.Fatal("big expired")
	}
	if c.Cost() != 8 {
		t.Fatalf("expired Get must release cost: %d", c.Cost())
	}
	c.Purge()
	if c.Cost() != 0 || c.Len() != 0 {
		t.Fatalf("after purge cost=%d len=%d", c.Cost(), c.Len())
	}
	// accounting stays right over many replacements
	for i := 0; i < 50; i++ {
		c.Set("k", i, int64(1+i%10), 0)
	}
	if c.Cost() != int64(1+49%10) {
		t.Fatalf("cost=%d after churn", c.Cost())
	}
}

func TestHiddenExpiredGoFirstWhenMakingRoom(t *testing.T) {
	var log []rec
	c, clk := hidden(6, 0, &log)
	c.Set("old", 1, 2, time.Second) // least recently used... after the Gets below it is not
	c.Set("keep1", 2, 2, 0)
	c.Set("keep2", 3, 2, 0)
	c.Get("old")   // old is now the most recently used, and about to expire
	c.Get("keep1") // order, newest first: keep1, old, keep2
	clk.Advance(2 * time.Second)
	log = nil
	c.Set("new", 4, 2, 0)
	want := []rec{{"old", Expired}}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("events = %v, want %v (the expired entry goes, live keep2 stays)", log, want)
	}
	if got := c.Keys(); !reflect.DeepEqual(got, []string{"new", "keep1", "keep2"}) {
		t.Fatalf("keys = %v", got)
	}
	st := c.Stats()
	if st.Evictions != 0 || st.Expirations != 1 {
		t.Fatalf("stats = %+v, want 0 evictions and 1 expiration", st)
	}
	// every expired entry is swept, not only as many as strictly needed
	c.Purge()
	c.Set("a", 0, 1, 0)
	c.Set("e1", 0, 1, time.Second)
	c.Set("e2", 0, 1, time.Second)
	c.Set("l", 0, 1, 0)
	clk.Advance(time.Second)
	log = nil
	c.Set("z", 0, 3, 0)
	if len(log) != 2 || log[0].reason != Expired || log[1].reason != Expired {
		t.Fatalf("events = %v, want both expired entries swept", log)
	}
	// otherwise live entries go, least recently used first
	log = nil
	c.Set("big", 0, 6, 0)
	if len(log) != 3 {
		t.Fatalf("events = %v", log)
	}
	order := []string{"a", "l", "z"}
	for i, r := range log {
		if r.key != order[i] || r.reason != Evicted {
			t.Fatalf("events = %v, want LRU order %v", log, order)
		}
	}
}

func TestHiddenOversizedAndNegativeAreRejectedWithoutSideEffects(t *testing.T) {
	var log []rec
	c, clk := hidden(5, 0, &log)
	c.Set("a", "va", 3, 0)
	c.Set("b", "vb", 2, time.Minute)
	c.Get("a")
	before := c.Stats()
	if c.Set("a", "huge", 6, 0) {
		t.Fatal("cost over capacity must be rejected")
	}
	if c.Set("b", "neg", -1, 0) {
		t.Fatal("negative cost must be rejected")
	}
	if c.Set("fresh", "huge", 100, 0) {
		t.Fatal("cost over capacity must be rejected")
	}
	if len(log) != 0 {
		t.Fatalf("rejected Set fired callbacks: %v", log)
	}
	if c.Stats() != before {
		t.Fatalf("rejected Set changed counters: %+v vs %+v", c.Stats(), before)
	}
	if v, ok := c.Peek("a"); !ok || v != "va" {
		t.Fatalf("a = %v, %v; the old value must survive a rejected Set", v, ok)
	}
	if c.Cost() != 5 || c.Len() != 2 {
		t.Fatalf("cost=%d len=%d", c.Cost(), c.Len())
	}
	if got := c.Keys(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("keys = %v", got)
	}
	// the surviving entry keeps its original expiry, too
	clk.Advance(time.Minute)
	if _, ok := c.Peek("b"); ok {
		t.Fatal("b should have expired on schedule")
	}
	// capacity-sized entries are fine
	if !c.Set("exact", 1, 5, 0) {
		t.Fatal("cost == capacity is allowed")
	}
}

func TestHiddenStatsAndCallbackReasons(t *testing.T) {
	var log []rec
	c, clk := hidden(3, 0, &log)
	c.Set("a", 1, 1, 5*time.Second)
	c.Set("b", 2, 1, 0)
	c.Set("b", 3, 1, 0) // replace a live entry
	c.Set("c", 4, 1, 0)
	c.Set("d", 5, 1, 0) // evicts a
	c.Get("zzz")
	c.Get("c")
	c.Delete("d")
	c.Set("e", 6, 1, 5*time.Second)
	clk.Advance(5 * time.Second)
	if n := c.Len(); n != 2 {
		t.Fatalf("Len = %d", n)
	}
	c.Set("f", 7, 1, 5*time.Second)
	clk.Advance(5 * time.Second)
	if n := c.Cost(); n != 2 {
		t.Fatalf("Cost = %d", n)
	}
	c.Set("g", 9, 1, 1) // 1ns TTL
	clk.Advance(1)
	c.Set("g", 10, 1, 0) // replaces an expired entry: Expired, not Replaced
	c.Purge()
	want := []rec{
		{"b", Replaced}, {"a", Evicted}, {"d", Removed},
		{"e", Expired}, {"f", Expired}, {"g", Expired},
		{"b", Removed}, {"c", Removed}, {"g", Removed},
	}
	if !reflect.DeepEqual(log, want) {
		t.Fatalf("events:\n got %v\nwant %v", log, want)
	}
	st := c.Stats()
	if st.Hits != 1 || st.Misses != 1 || st.Evictions != 1 || st.Expirations != 3 {
		t.Fatalf("stats = %+v, want hits 1 misses 1 evictions 1 expirations 3", st)
	}
}

func TestHiddenCallbacksRunUnlockedAndConcurrencyIsSafe(t *testing.T) {
	done := make(chan string, 1)
	go func() {
		var c *Cache
		var lens []int
		clk := NewFakeClock(h0)
		c = New(Config{Capacity: 2, Clock: clk, OnRemove: func(k string, v any, r Reason) {
			lens = append(lens, c.Len()) // re-enters the cache
			c.Peek(k)
		}})
		c.Set("a", 1, 1, 0)
		c.Set("b", 2, 1, 0)
		c.Set("c", 3, 1, 0) // evicts a
		c.Delete("b")
		c.Set("d", 4, 1, time.Second)
		clk.Advance(time.Second)
		c.Get("d")
		if want := []int{2, 1, 1}; !reflect.DeepEqual(lens, want) {
			done <- fmt.Sprintf("Len seen from callbacks = %v, want %v", lens, want)
			return
		}
		done <- ""
	}()
	select {
	case msg := <-done:
		if msg != "" {
			t.Fatal(msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: OnRemove must run after the lock is released")
	}

	// concurrent use; run with -race
	c := New(Config{Capacity: 50, DefaultTTL: time.Hour})
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				k := fmt.Sprintf("k%d", (g*7+i)%80)
				c.Set(k, i, int64(1+i%3), 0)
				c.Get(k)
				c.Peek(k)
				if i%50 == 0 {
					c.Delete(k)
					c.Len()
					c.Keys()
					c.Stats()
				}
			}
		}(g)
	}
	wg.Wait()
	if c.Cost() > 50 {
		t.Fatalf("cost %d over capacity", c.Cost())
	}
}
