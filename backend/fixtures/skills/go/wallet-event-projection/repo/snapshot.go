package wallet

// Snapshot is a frozen, independent copy of a Projection, buffered events
// included. Changing the projection afterwards does not change the snapshot,
// and projections restored from one snapshot do not affect each other.
type Snapshot struct {
	aggs map[string]aggSnapshot
}

type aggSnapshot struct {
	state   State
	history []Event
	pending []Event
}

// Snapshot captures the projection.
func (p *Projection) Snapshot() Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := Snapshot{aggs: make(map[string]aggSnapshot, len(p.aggs))}
	for id, a := range p.aggs {
		as := aggSnapshot{
			state:   a.state.clone(),
			history: a.history,
		}
		for _, ev := range a.pending {
			as.pending = append(as.pending, ev)
		}
		s.aggs[id] = as
	}
	return s
}

// Restore builds a new projection from a snapshot; feeding it the rest of the
// stream gives the same result as never having stopped.
func Restore(s Snapshot) *Projection {
	p := NewProjection()
	for id, as := range s.aggs {
		a := p.agg(id)
		a.state = as.state.clone()
		a.history = as.history
		for _, ev := range as.pending {
			a.pending[ev.Seq] = ev
		}
	}
	return p
}
