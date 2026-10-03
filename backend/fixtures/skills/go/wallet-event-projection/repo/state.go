package wallet

// Rejection records an event that was valid but broke a business rule.
type Rejection struct {
	Seq    uint64
	Reason string
}

// State is the projection of one wallet.
type State struct {
	ID       string
	Name     string
	Balance  int64
	Opened   bool
	Closed   bool
	Version  uint64 // sequence number of the last event dealt with, accepted or rejected
	Rejected []Rejection
}

// clone returns a deep copy.
func (s State) clone() State {
	s.Rejected = append([]Rejection(nil), s.Rejected...)
	return s
}

// apply deals with the event that follows s.Version and returns the new state.
// The state passed in is not modified. An event that breaks a rule leaves the
// wallet as it was, is recorded in Rejected, and still advances Version.
//
// Rules, checked in this order:
//   - the first event of a wallet must be Open, and Open is only valid first
//     ("not opened" / "already opened");
//   - once closed, every event is rejected ("closed");
//   - Open needs Amount >= 0 ("negative opening balance");
//   - Deposit and Withdraw need Amount > 0 ("invalid amount");
//   - Withdraw needs Balance >= Amount ("insufficient funds");
//   - Close needs a zero balance ("balance not zero");
//   - Rename needs a non-empty name ("empty name").
func apply(s State, ev Event) State {
	s = s.clone()
	s.ID = ev.Aggregate
	s.Version = ev.Seq
	reject := func(reason string) State {
		s.Rejected = append(s.Rejected, Rejection{Seq: ev.Seq, Reason: reason})
		s.Version = ev.Seq - 1
		return s
	}
	switch {
	case !s.Opened && ev.Kind != Open:
		return reject("not opened")
	case s.Opened && ev.Kind == Open:
		return reject("already opened")
	case s.Closed && ev.Kind != Rename:
		return reject("closed")
	}
	switch ev.Kind {
	case Open:
		if ev.Amount < 0 {
			return reject("negative opening balance")
		}
		s.Opened = true
		s.Name = ev.Name
		s.Balance = ev.Amount
	case Deposit:
		if ev.Amount <= 0 {
			return reject("invalid amount")
		}
		s.Balance += ev.Amount
	case Withdraw:
		if ev.Amount <= 0 {
			return reject("invalid amount")
		}
		if s.Balance <= ev.Amount {
			return reject("insufficient funds")
		}
		s.Balance -= ev.Amount
	case Close:
		if s.Balance != 0 {
			return reject("balance not zero")
		}
		s.Closed = true
	case Rename:
		if ev.Name == "" {
			return reject("empty name")
		}
		s.Name = ev.Name
	}
	return s
}
