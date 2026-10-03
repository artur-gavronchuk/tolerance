package batcher

import (
	"reflect"
	"testing"
	"time"
)

func collect() (*[][]string, func([]string)) {
	var got [][]string
	return &got, func(b []string) { got = append(got, append([]string(nil), b...)) }
}

func TestSizeTrigger(t *testing.T) {
	got, flush := collect()
	b := New(Config{MaxItems: 2, Flush: flush})
	b.Add("a")
	b.Add("b")
	b.Add("c")
	if want := [][]string{{"a", "b"}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %v want %v", *got, want)
	}
}

func TestDelayTrigger(t *testing.T) {
	got, flush := collect()
	clk := NewFakeClock()
	b := New(Config{MaxDelay: 100 * time.Millisecond, Flush: flush, Clock: clk})
	b.Add("a")
	clk.Advance(99 * time.Millisecond)
	if len(*got) != 0 {
		t.Fatalf("flushed early: %v", *got)
	}
	clk.Advance(time.Millisecond)
	if want := [][]string{{"a"}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %v want %v", *got, want)
	}
}

func TestCloseFlushes(t *testing.T) {
	got, flush := collect()
	b := New(Config{MaxItems: 10, Flush: flush})
	b.Add("a")
	b.Close()
	if want := [][]string{{"a"}}; !reflect.DeepEqual(*got, want) {
		t.Fatalf("got %v want %v", *got, want)
	}
	if err := b.Add("x"); err != ErrClosed {
		t.Fatalf("add after close: %v", err)
	}
}
