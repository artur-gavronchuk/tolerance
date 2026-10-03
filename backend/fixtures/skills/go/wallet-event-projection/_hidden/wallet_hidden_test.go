package wallet

import (
	"errors"
	"math/rand"
	"reflect"
	"sync"
	"testing"
	"time"
)

var hb = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func he(id string, seq uint64, kind Kind, amount int64, name string, min int) Event {
	return Event{Aggregate: id, Seq: seq, Kind: kind, Amount: amount, Name: name, At: hb.Add(time.Duration(min) * time.Minute)}
}

func feed(t *testing.T, p *Projection, evs ...Event) {
	t.Helper()
	for _, e := range evs {
		if err := p.Handle(e); err != nil {
			t.Fatalf("Handle(%+v): %v", e, err)
		}
	}
}

// a script that exercises every rule; seqs 1..12
func script(id string) []Event {
	return []Event{
		he(id, 1, Open, 100, "main", 1),
		he(id, 2, Withdraw, 150, "", 2), // insufficient funds
		he(id, 3, Deposit, 0, "", 3),    // invalid amount
		he(id, 4, Withdraw, 100, "", 4), // the whole balance is fine
		he(id, 5, Rename, 0, "", 5),     // empty name
		he(id, 6, Rename, 0, "spare", 6),
		he(id, 7, Deposit, 20, "", 7),
		he(id, 8, Close, 0, "", 8), // balance not zero
		he(id, 9, Withdraw, 20, "", 9),
		he(id, 10, Close, 0, "", 10),
		he(id, 11, Rename, 0, "late", 11), // closed
		he(id, 12, Deposit, 5, "", 12),    // closed
	}
}

func TestHiddenRejectedEventsAdvanceTheVersion(t *testing.T) {
	p := NewProjection()
	feed(t, p, script("w")...)
	s, ok := p.Get("w")
	if !ok {
		t.Fatal("no state")
	}
	want := State{
		ID: "w", Name: "spare", Balance: 0, Opened: true, Closed: true, Version: 12,
		Rejected: []Rejection{
			{2, "insufficient funds"}, {3, "invalid amount"}, {5, "empty name"},
			{8, "balance not zero"}, {11, "closed"}, {12, "closed"},
		},
	}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("state:\n got %+v\nwant %+v", s, want)
	}
	// first event must be Open; Open only first; negative opening balance
	q := NewProjection()
	feed(t, q, he("x", 1, Deposit, 5, "", 1), he("x", 2, Open, -1, "n", 2), he("x", 3, Open, 7, "n", 3), he("x", 4, Open, 9, "z", 4))
	s, _ = q.Get("x")
	want = State{ID: "x", Name: "n", Balance: 7, Opened: true, Version: 4, Rejected: []Rejection{
		{1, "not opened"}, {2, "negative opening balance"}, {4, "already opened"},
	}}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("state:\n got %+v\nwant %+v", s, want)
	}
}

func TestHiddenOutOfOrderDeliveryDrainsEverything(t *testing.T) {
	evs := script("w")
	want := NewProjection()
	feed(t, want, evs...)
	ws, _ := want.Get("w")

	// reversed: all but seq 1 are buffered, then one event releases all of them
	p := NewProjection()
	for i := len(evs) - 1; i >= 1; i-- {
		feed(t, p, evs[i])
	}
	if p.Pending("w") != 11 {
		t.Fatalf("pending = %d, want 11", p.Pending("w"))
	}
	if _, ok := p.Get("w"); ok {
		t.Fatal("no state before seq 1 is applied")
	}
	feed(t, p, evs[0])
	got, _ := p.Get("w")
	if !reflect.DeepEqual(got, ws) || p.Pending("w") != 0 {
		t.Fatalf("got %+v pending %d\nwant %+v", got, p.Pending("w"), ws)
	}

	// a gap that stays open keeps the tail buffered, and filling it later releases the tail
	p = NewProjection()
	feed(t, p, evs[0], evs[1], evs[2], evs[4], evs[5], evs[6])
	got, _ = p.Get("w")
	if got.Version != 3 || p.Pending("w") != 3 {
		t.Fatalf("version %d pending %d, want 3 and 3", got.Version, p.Pending("w"))
	}
	feed(t, p, evs[3])
	got, _ = p.Get("w")
	if got.Version != 7 || p.Pending("w") != 0 {
		t.Fatalf("version %d pending %d, want 7 and 0", got.Version, p.Pending("w"))
	}
}

func TestHiddenDuplicatesAndConflicts(t *testing.T) {
	p := NewProjection()
	loc := time.FixedZone("x", 3*3600)
	feed(t, p, he("w", 1, Open, 10, "a", 0), he("w", 2, Deposit, 5, "", 1), he("w", 4, Deposit, 100, "", 3))
	// exact copies are ignored, wherever the original is; an equal instant in another zone is the same event
	same := he("w", 2, Deposit, 5, "", 1)
	same.At = same.At.In(loc)
	feed(t, p, he("w", 1, Open, 10, "a", 0), same, he("w", 4, Deposit, 100, "", 3))
	if p.Pending("w") != 1 {
		t.Fatalf("pending = %d", p.Pending("w"))
	}
	// different content under a taken Seq is a conflict, applied or buffered, and changes nothing
	for _, bad := range []Event{
		he("w", 1, Open, 11, "a", 0), he("w", 2, Deposit, 6, "", 1), he("w", 2, Withdraw, 5, "", 1),
		he("w", 2, Deposit, 5, "", 2), he("w", 4, Deposit, 101, "", 3), he("w", 4, Deposit, 100, "n", 3),
	} {
		if err := p.Handle(bad); !errors.Is(err, ErrConflict) {
			t.Fatalf("Handle(%+v) = %v, want ErrConflict", bad, err)
		}
	}
	feed(t, p, he("w", 3, Deposit, 1, "", 2))
	s, _ := p.Get("w")
	if s.Balance != 116 || s.Version != 4 {
		t.Fatalf("state = %+v: the first event received for a Seq must win", s)
	}
	// invalid events are errors and leave no trace
	for _, bad := range []Event{he("", 1, Open, 1, "", 0), he("w", 0, Open, 1, "", 0), he("w", 5, "refund", 1, "", 0)} {
		if err := p.Handle(bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("Handle(%+v) = %v, want ErrInvalid", bad, err)
		}
	}
	if p.Pending("w") != 0 {
		t.Fatal("invalid events must not be buffered")
	}
	feed(t, p, he("w", 5, Deposit, 4, "", 4))
	if s, _ := p.Get("w"); s.Balance != 120 {
		t.Fatalf("balance = %d", s.Balance)
	}
}

func TestHiddenAsOfFollowsSeqOrderNotClockOrder(t *testing.T) {
	p := NewProjection()
	feed(t, p,
		he("w", 1, Open, 100, "a", 10),
		he("w", 2, Deposit, 50, "", 20),
		he("w", 3, Withdraw, 120, "", 15), // clock drift: stamped before seq 2
		he("w", 4, Deposit, 1, "", 30),
		he("w", 6, Deposit, 1000, "", 5), // buffered: never part of the history
	)
	at := func(m int) (State, bool) { return p.AsOf("w", hb.Add(time.Duration(m)*time.Minute)) }
	if _, ok := at(9); ok {
		t.Fatal("nothing at minute 9")
	}
	s, ok := at(10)
	if !ok || s.Balance != 100 || s.Version != 1 {
		t.Fatalf("t=10: %+v", s)
	}
	// at minute 16 seq 3 (stamped 15) is dated before t but seq 2 (stamped 20) is not: the prefix ends at seq 1
	s, _ = at(16)
	if s.Balance != 100 || s.Version != 1 {
		t.Fatalf("t=16: %+v, want the prefix ending at seq 1", s)
	}
	s, _ = at(20)
	if s.Balance != 30 || s.Version != 3 {
		t.Fatalf("t=20: %+v, want seq 1..3 (balance 30)", s)
	}
	s, _ = at(1000)
	if s.Balance != 31 || s.Version != 4 {
		t.Fatalf("t=1000: %+v", s)
	}
	live, _ := p.Get("w")
	if !reflect.DeepEqual(s, live) {
		t.Fatalf("AsOf(far future) = %+v differs from Get = %+v", s, live)
	}
	// rejected events show in AsOf too
	feed(t, p, he("w", 5, Withdraw, 9999, "", 40))
	s, _ = at(45)
	if s.Version != 6 || len(s.Rejected) != 1 || s.Rejected[0].Seq != 5 || s.Balance != 1031 {
		t.Fatalf("t=45: %+v", s)
	}
	if _, ok := p.AsOf("nobody", hb); ok {
		t.Fatal("unknown wallet")
	}
}

func TestHiddenSnapshotsAreIndependent(t *testing.T) {
	evs := script("w")
	p := NewProjection()
	feed(t, p, evs[:7]...)
	feed(t, p, evs[9], evs[10]) // seq 10 and 11 buffered
	snap := p.Snapshot()

	a, b := Restore(snap), Restore(snap)
	feed(t, a, evs[7:]...)                                                        // seq 8, 9 release the buffered 10, 11 and a new 12
	feed(t, b, he("w", 8, Deposit, 1, "", 8), he("w", 9, Withdraw, 99999, "", 9)) // diverges from a on purpose
	feed(t, p, evs[7:]...)

	sa, _ := a.Get("w")
	sp, _ := p.Get("w")
	if !reflect.DeepEqual(sa, sp) {
		t.Fatalf("restored run differs from the uninterrupted one:\n got %+v\nwant %+v", sa, sp)
	}
	sb, _ := b.Get("w")
	if sb.Version != 11 || sb.Balance != 21 || len(sb.Rejected) != 5 || sb.Closed { // rejected: 2, 3, 5, 9, 10
		t.Fatalf("b = %+v", sb)
	}
	// histories must be separate: AsOf and conflict detection read them
	if s, _ := b.AsOf("w", hb.Add(9*time.Minute)); s.Balance != 21 || s.Version != 9 {
		t.Fatalf("b asof = %+v", s)
	}
	if s, _ := a.AsOf("w", hb.Add(9*time.Minute)); s.Balance != 0 || s.Version != 9 {
		t.Fatalf("a asof = %+v", s)
	}
	if err := a.Handle(he("w", 8, Close, 0, "", 8)); err != nil {
		t.Fatalf("a: copy of seq 8: %v", err)
	}
	if err := a.Handle(he("w", 9, Withdraw, 99999, "", 9)); !errors.Is(err, ErrConflict) {
		t.Fatalf("a must still hold the original seq 9: %v", err)
	}
	if err := b.Handle(he("w", 8, Close, 0, "", 8)); !errors.Is(err, ErrConflict) {
		t.Fatalf("b must hold its own seq 8: %v", err)
	}
	// the original is not affected by what the restores did, and the snapshot is reusable
	c := Restore(snap)
	if s, _ := c.Get("w"); s.Version != 7 || c.Pending("w") != 2 {
		t.Fatalf("c = %+v pending %d", s, c.Pending("w"))
	}
	// state returned by Get is a copy
	s, _ := p.Get("w")
	s.Rejected[0].Reason = "tampered"
	if s2, _ := p.Get("w"); s2.Rejected[0].Reason != "insufficient funds" {
		t.Fatal("Get must return a copy")
	}
}

func TestHiddenShuffledDeliveryEqualsInOrderReplay(t *testing.T) {
	ids := []string{"a", "b", "c", "d", "e"}
	var all []Event
	ref := NewProjection()
	for i, id := range ids {
		evs := script(id)
		if i%2 == 1 {
			evs = evs[:9]
		}
		feed(t, ref, evs...)
		all = append(all, evs...)
	}
	rng := rand.New(rand.NewSource(7))
	for round := 0; round < 20; round++ {
		in := append([]Event(nil), all...)
		rng.Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
		// duplicates sprinkled in
		for k := 0; k < 15; k++ {
			in = append(in, all[rng.Intn(len(all))])
		}
		rng.Shuffle(len(in), func(i, j int) { in[i], in[j] = in[j], in[i] })
		p := NewProjection()
		var wg sync.WaitGroup
		chunks := 4
		for c := 0; c < chunks; c++ {
			wg.Add(1)
			go func(part []Event) {
				defer wg.Done()
				for _, e := range part {
					if err := p.Handle(e); err != nil {
						t.Errorf("Handle: %v", err)
					}
				}
			}(in[c*len(in)/chunks : (c+1)*len(in)/chunks])
		}
		wg.Wait()
		if !reflect.DeepEqual(p.IDs(), ref.IDs()) {
			t.Fatalf("round %d: ids %v", round, p.IDs())
		}
		for _, id := range ids {
			got, _ := p.Get(id)
			want, _ := ref.Get(id)
			if !reflect.DeepEqual(got, want) || p.Pending(id) != 0 {
				t.Fatalf("round %d wallet %s:\n got %+v pending %d\nwant %+v", round, id, got, p.Pending(id), want)
			}
		}
	}
}
