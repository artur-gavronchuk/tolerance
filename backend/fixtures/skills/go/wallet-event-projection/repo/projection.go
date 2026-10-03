package wallet

import (
	"sort"
	"sync"
)

// aggregate is everything the projection knows about one wallet.
type aggregate struct {
	state   State
	history []Event          // applied events; history[i].Seq == i+1
	pending map[uint64]Event // received ahead of their turn, by Seq
}

// Projection builds wallet states from an event stream that may arrive out of
// order and with duplicates. It is safe for concurrent use.
type Projection struct {
	mu   sync.Mutex
	aggs map[string]*aggregate
}

// NewProjection returns an empty projection.
func NewProjection() *Projection {
	return &Projection{aggs: make(map[string]*aggregate)}
}

func (p *Projection) agg(id string) *aggregate {
	a, ok := p.aggs[id]
	if !ok {
		a = &aggregate{pending: make(map[uint64]Event)}
		p.aggs[id] = a
	}
	return a
}

// Handle feeds one event to the projection.
//
//   - A malformed event returns ErrInvalid and is not recorded.
//   - An event with Seq == Version+1 is applied, and then every buffered
//     event that has become the next in line is applied too, until there is a gap.
//   - An event ahead of its turn is buffered until the gap before it is filled.
//   - A copy of an event already applied or already buffered is ignored (nil).
//     A different event under a Seq that is taken, applied or buffered, returns
//     ErrConflict and changes nothing; the first one received stays.
func (p *Projection) Handle(ev Event) error {
	if err := ev.validate(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	a := p.agg(ev.Aggregate)
	if ev.Seq <= a.state.Version {
		return nil
	}
	a.pending[ev.Seq] = ev
	for {
		next, ok := a.pending[a.state.Version+1]
		if !ok {
			return nil
		}
		delete(a.pending, next.Seq)
		rejected := len(a.state.Rejected)
		a.state = apply(a.state, next)
		a.history = append(a.history, next)
		if len(a.state.Rejected) > rejected {
			return nil // a rejected event ends the run
		}
	}
}

// Get returns the state of a wallet as built from the events applied so far.
func (p *Projection) Get(id string) (State, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	a, ok := p.aggs[id]
	if !ok || a.state.Version == 0 {
		return State{}, false
	}
	return a.state.clone(), true
}

// Pending is the number of events buffered for a wallet, waiting for a gap to close.
func (p *Projection) Pending(id string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if a, ok := p.aggs[id]; ok {
		return len(a.pending)
	}
	return 0
}

// IDs lists the wallets that have at least one applied event, sorted.
func (p *Projection) IDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var ids []string
	for id, a := range p.aggs {
		if a.state.Version > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}
