package wallet

import (
	"testing"
	"time"
)

var base = time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)

func ev(id string, seq uint64, kind Kind, amount int64) Event {
	return Event{Aggregate: id, Seq: seq, At: base.Add(time.Duration(seq) * time.Minute), Kind: kind, Amount: amount, Name: "main"}
}

func mustHandle(t *testing.T, p *Projection, e Event) {
	t.Helper()
	if err := p.Handle(e); err != nil {
		t.Fatalf("Handle(%+v): %v", e, err)
	}
}

func TestInOrder(t *testing.T) {
	p := NewProjection()
	mustHandle(t, p, ev("w1", 1, Open, 100))
	mustHandle(t, p, ev("w1", 2, Deposit, 50))
	mustHandle(t, p, ev("w1", 3, Withdraw, 30))
	s, ok := p.Get("w1")
	if !ok || s.Balance != 120 || s.Version != 3 || s.Name != "main" {
		t.Fatalf("state = %+v, %v", s, ok)
	}
}

func TestOutOfOrderIsBuffered(t *testing.T) {
	p := NewProjection()
	mustHandle(t, p, ev("w1", 3, Deposit, 5))
	mustHandle(t, p, ev("w1", 2, Deposit, 10))
	if _, ok := p.Get("w1"); ok {
		t.Fatal("nothing can be applied before seq 1")
	}
	if p.Pending("w1") != 2 {
		t.Fatalf("pending = %d", p.Pending("w1"))
	}
	mustHandle(t, p, ev("w1", 1, Open, 1))
	s, _ := p.Get("w1")
	if s.Balance != 16 || s.Version != 3 || p.Pending("w1") != 0 {
		t.Fatalf("state = %+v pending %d", s, p.Pending("w1"))
	}
}

func TestDuplicateIgnored(t *testing.T) {
	p := NewProjection()
	mustHandle(t, p, ev("w1", 1, Open, 10))
	mustHandle(t, p, ev("w1", 1, Open, 10))
	mustHandle(t, p, ev("w1", 2, Deposit, 5))
	mustHandle(t, p, ev("w1", 2, Deposit, 5))
	if s, _ := p.Get("w1"); s.Balance != 15 {
		t.Fatalf("balance = %d", s.Balance)
	}
}

func TestInvalidEvents(t *testing.T) {
	p := NewProjection()
	for _, e := range []Event{{Seq: 1, Kind: Open}, {Aggregate: "w", Kind: Open}, {Aggregate: "w", Seq: 1, Kind: "bogus"}} {
		if err := p.Handle(e); err == nil {
			t.Fatalf("Handle(%+v) should fail", e)
		}
	}
}

func TestSnapshotRestore(t *testing.T) {
	p := NewProjection()
	mustHandle(t, p, ev("w1", 1, Open, 10))
	mustHandle(t, p, ev("w1", 2, Deposit, 5))
	snap := p.Snapshot()
	q := Restore(snap)
	mustHandle(t, q, ev("w1", 3, Deposit, 1))
	if s, _ := q.Get("w1"); s.Balance != 16 {
		t.Fatalf("restored balance = %d", s.Balance)
	}
	if s, _ := p.Get("w1"); s.Balance != 15 {
		t.Fatalf("original balance = %d", s.Balance)
	}
}

func TestAsOfSimple(t *testing.T) {
	p := NewProjection()
	mustHandle(t, p, ev("w1", 1, Open, 10))
	mustHandle(t, p, ev("w1", 2, Deposit, 5))
	mustHandle(t, p, ev("w1", 3, Deposit, 7))
	s, ok := p.AsOf("w1", base.Add(2*time.Minute))
	if !ok || s.Balance != 15 {
		t.Fatalf("asof = %+v, %v", s, ok)
	}
	if _, ok := p.AsOf("w1", base); ok {
		t.Fatal("nothing existed at base")
	}
}
