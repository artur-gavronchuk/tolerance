package wallet

import (
	"errors"
	"fmt"
	"time"
)

// Kind is the type of a wallet event.
type Kind string

const (
	Open     Kind = "open"     // Amount is the opening balance (>= 0); Name the wallet's name
	Deposit  Kind = "deposit"  // Amount > 0 is added
	Withdraw Kind = "withdraw" // Amount > 0 is taken out
	Close    Kind = "close"    // closes the wallet; its balance must be zero
	Rename   Kind = "rename"   // Name is the new name
)

var (
	// ErrInvalid is returned by Handle for a malformed event. Nothing is recorded.
	ErrInvalid = errors.New("wallet: invalid event")
	// ErrConflict is returned by Handle when an event claims a sequence number
	// that already holds a different event.
	ErrConflict = errors.New("wallet: conflicting event for sequence number")
)

// Event is one fact about a wallet. Events of one wallet (Aggregate) are
// numbered 1, 2, 3, ... by Seq and must be applied in that order, but the
// transport delivers them in any order, and more than once.
type Event struct {
	Aggregate string
	Seq       uint64
	At        time.Time
	Kind      Kind
	Amount    int64
	Name      string
}

func (e Event) validate() error {
	if e.Aggregate == "" {
		return fmt.Errorf("%w: empty aggregate", ErrInvalid)
	}
	if e.Seq == 0 {
		return fmt.Errorf("%w: sequence numbers start at 1", ErrInvalid)
	}
	switch e.Kind {
	case Open, Deposit, Withdraw, Close, Rename:
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalid, e.Kind)
	}
	return nil
}

// same reports whether two events are the same fact (the same instant counts
// as equal whatever the location).
func (e Event) same(o Event) bool {
	return e.Aggregate == o.Aggregate && e.Seq == o.Seq && e.At.Equal(o.At) &&
		e.Kind == o.Kind && e.Amount == o.Amount && e.Name == o.Name
}
