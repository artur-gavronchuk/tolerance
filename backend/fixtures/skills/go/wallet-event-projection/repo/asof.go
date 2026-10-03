package wallet

import "time"

// AsOf returns the state of a wallet as it was at time t: the events are
// applied in Seq order, stopping before the first applied event whose At is
// after t. (Clocks drift, so an event may carry an earlier time than its
// predecessor; the wallet's own order wins, and a later event never takes
// effect before the events that precede it.) Buffered events, which have not
// been applied, play no part. The second result is false when no event
// qualifies.
func (p *Projection) AsOf(id string, t time.Time) (State, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	a, ok := p.aggs[id]
	if !ok {
		return State{}, false
	}
	var s State
	n := 0
	for _, ev := range a.history {
		if ev.At.After(t) {
			continue
		}
		s = apply(s, ev)
		n++
	}
	if n == 0 {
		return State{}, false
	}
	return s, true
}
