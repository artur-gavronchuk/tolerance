package batcher

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu      sync.Mutex
	batches [][]string
}

func (r *recorder) flush(b []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batches = append(r.batches, b) // keeps the slice on purpose
}

func (r *recorder) all() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]string(nil), r.batches...)
}

// manualClock records timer callbacks and lets the test run them at will,
// including after the batch they belonged to was cut.
type manualClock struct {
	mu  sync.Mutex
	fns []func()
}

type manualTimer struct{}

func (manualTimer) Stop() bool { return false } // "already running"

func (c *manualClock) Now() time.Time { return time.Unix(0, 0) }
func (c *manualClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fns = append(c.fns, f)
	return manualTimer{}
}
func (c *manualClock) fn(i int) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.fns[i]
}
func (c *manualClock) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.fns)
}

func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: did not return (deadlock?)", what)
	}
}

func TestDelayIsMeasuredFromFirstItem(t *testing.T) {
	r := &recorder{}
	clk := NewFakeClock()
	b := New(Config{MaxDelay: 100 * time.Millisecond, Flush: r.flush, Clock: clk})
	b.Add("a")
	clk.Advance(60 * time.Millisecond)
	b.Add("b")
	clk.Advance(39 * time.Millisecond)
	if n := len(r.all()); n != 0 {
		t.Fatalf("flushed at 99ms: %v", r.all())
	}
	clk.Advance(time.Millisecond)
	if want := [][]string{{"a", "b"}}; !reflect.DeepEqual(r.all(), want) {
		t.Fatalf("got %v want %v", r.all(), want)
	}
	// the next batch gets a fresh full delay from its own first item
	clk.Advance(500 * time.Millisecond)
	b.Add("c")
	clk.Advance(99 * time.Millisecond)
	if len(r.all()) != 1 {
		t.Fatalf("second batch flushed early: %v", r.all())
	}
	clk.Advance(time.Millisecond)
	if want := [][]string{{"a", "b"}, {"c"}}; !reflect.DeepEqual(r.all(), want) {
		t.Fatalf("got %v want %v", r.all(), want)
	}
}

func TestStaleTimerDoesNothing(t *testing.T) {
	r := &recorder{}
	clk := &manualClock{}
	b := New(Config{MaxItems: 2, MaxDelay: time.Second, Flush: r.flush, Clock: clk})
	b.Add("a")
	b.Add("b") // size trigger cuts [a b]; the timer armed by "a" is now stale
	b.Add("c")
	if clk.count() < 2 {
		t.Fatalf("expected a timer per batch, got %d", clk.count())
	}
	clk.fn(0)() // stale callback of the first batch
	if want := [][]string{{"a", "b"}}; !reflect.DeepEqual(r.all(), want) {
		t.Fatalf("stale timer cut the next batch: %v", r.all())
	}
	// the live timer of batch two must still work
	clk.fn(clk.count() - 1)()
	if want := [][]string{{"a", "b"}, {"c"}}; !reflect.DeepEqual(r.all(), want) {
		t.Fatalf("live timer: %v", r.all())
	}
	// and now it is stale itself
	b.Add("d")
	clk.fn(clk.count() - 2)()
	if len(r.all()) != 2 {
		t.Fatalf("second stale timer flushed: %v", r.all())
	}
}

func TestFlushMayCallAdd(t *testing.T) {
	r := &recorder{}
	var b *Batcher
	first := true
	b = New(Config{MaxItems: 2, Flush: func(batch []string) {
		r.flush(batch)
		if first {
			first = false
			if err := b.Add("x"); err != nil {
				t.Errorf("add from flush: %v", err)
			}
		}
	}})
	within(t, "reentrant add", func() {
		b.Add("a")
		b.Add("b")
		b.Close()
	})
	if want := [][]string{{"a", "b"}, {"x"}}; !reflect.DeepEqual(r.all(), want) {
		t.Fatalf("got %v want %v", r.all(), want)
	}
}

func TestBatchesAreNotAliased(t *testing.T) {
	r := &recorder{}
	b := New(Config{MaxItems: 2, Flush: r.flush})
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		b.Add(s)
	}
	b.Close()
	if want := [][]string{{"a", "b"}, {"c", "d"}, {"e"}}; !reflect.DeepEqual(r.all(), want) {
		t.Fatalf("got %v want %v", r.all(), want)
	}
}

func TestSerialOrderedDeliveryWithoutBlockingAdd(t *testing.T) {
	r := &recorder{}
	started := make(chan struct{})
	release := make(chan struct{})
	var mu sync.Mutex
	inflight, maxInflight := 0, 0
	var once sync.Once
	b := New(Config{MaxItems: 2, Flush: func(batch []string) {
		mu.Lock()
		inflight++
		if inflight > maxInflight {
			maxInflight = inflight
		}
		mu.Unlock()
		once.Do(func() { close(started); <-release })
		r.flush(batch)
		mu.Lock()
		inflight--
		mu.Unlock()
	}})
	go func() { b.Add("a"); b.Add("b") }() // delivers batch 1 and blocks in Flush
	<-started
	within(t, "add while another goroutine delivers", func() {
		b.Add("c")
		b.Add("d") // cuts batch 2; must be queued, not delivered concurrently
		b.Add("e")
		b.Add("f")
	})
	if n := len(r.all()); n != 0 {
		t.Fatalf("delivered while batch 1 still in flight: %v", r.all())
	}
	close(release)
	within(t, "close", b.Close)
	want := [][]string{{"a", "b"}, {"c", "d"}, {"e", "f"}}
	if !reflect.DeepEqual(r.all(), want) {
		t.Fatalf("got %v want %v", r.all(), want)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxInflight != 1 {
		t.Fatalf("Flush ran %d at a time", maxInflight)
	}
}

func TestCloseWaitsForInFlightAndLimitsOff(t *testing.T) {
	r := &recorder{}
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	b := New(Config{MaxItems: 2, Flush: func(batch []string) {
		once.Do(func() { close(started); <-release })
		r.flush(batch)
	}})
	go func() { b.Add("a"); b.Add("b") }()
	<-started
	within(t, "add while another goroutine delivers", func() { b.Add("c") })
	closed := make(chan struct{})
	go func() { b.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned while a batch was still being delivered")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not return")
	}
	if want := [][]string{{"a", "b"}, {"c"}}; !reflect.DeepEqual(r.all(), want) {
		t.Fatalf("got %v want %v", r.all(), want)
	}
	if err := b.Add("z"); err != ErrClosed {
		t.Fatalf("add after close: %v", err)
	}
	b.Close() // no-op, no empty batch

	// no limits: nothing flushes until Close, and an empty Close delivers nothing
	r2 := &recorder{}
	clk := NewFakeClock()
	b2 := New(Config{Flush: r2.flush, Clock: clk})
	for i := 0; i < 50; i++ {
		b2.Add("x")
	}
	clk.Advance(time.Hour)
	if len(r2.all()) != 0 {
		t.Fatalf("flushed without limits: %d", len(r2.all()))
	}
	b2.Close()
	if got := r2.all(); len(got) != 1 || len(got[0]) != 50 {
		t.Fatalf("close with no limits: %v", got)
	}
	r3 := &recorder{}
	New(Config{Flush: r3.flush}).Close()
	if len(r3.all()) != 0 {
		t.Fatal("empty batch delivered")
	}
}
