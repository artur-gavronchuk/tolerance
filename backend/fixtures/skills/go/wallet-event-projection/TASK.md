# Wallet event projection

Package `wallet` projects a stream of wallet events into per-wallet `State`.
The transport delivers events in any order and more than once. The visible
tests pass, but production states drift, some wallets get stuck, and
point-in-time queries disagree with the live state. Fix the package without
editing `*_test.go` files or adding dependencies. Hidden tests exercise the
contract below.

## Events and ordering

The events of one wallet (`Aggregate`) carry `Seq` 1, 2, 3, ... and are applied
strictly in that order. `Handle(ev)`:

- A malformed event (empty `Aggregate`, `Seq == 0`, unknown `Kind`) returns an
  error wrapping `ErrInvalid` and is not recorded in any way.
- `Seq == Version+1`: applied, and then every buffered event that has become the
  next in line is applied as well, until the next one is missing.
- `Seq > Version+1`: buffered (`Pending` counts them) until the gap before it is filled.
- An event that is a copy of one already applied or buffered (all fields equal;
  `At` compares as an instant) is ignored and `Handle` returns nil.
- A different event with the `Seq` of one already applied or buffered returns
  `ErrConflict` and changes nothing: the first event received for a `Seq` stays.

`Get(id)` returns the state built from applied events only; it reports `false`
until the wallet's first event has been applied. The state is a copy.

## Rules and rejected events

Rules are checked in this order, and the first that fails rejects the event
(see `apply` in `state.go` for the reasons): the first event must be `Open`
and `Open` is only valid first; once `Closed` every event is rejected;
`Open` needs `Amount >= 0`; `Deposit` and `Withdraw` need `Amount > 0`;
`Withdraw` needs `Balance >= Amount` (withdrawing the whole balance is fine);
`Close` needs a zero balance; `Rename` needs a non-empty name.

A rejected event is **not an error**. It is a fact like any other: the wallet is
left unchanged, a `Rejection{Seq, Reason}` is appended to `State.Rejected`, and
`Version` advances to its `Seq` so that the events behind it can be applied.
It does not stop the application of events buffered behind it.

## AsOf

`AsOf(id, t)` returns the state as it was at `t`: the applied events of the
wallet are applied in `Seq` order, stopping before the first event whose `At` is
after `t`. Timestamps can go backwards (clock drift): an event that is earlier
than its predecessor is still applied after it, and an event never takes effect
before the events that precede it, whatever its own `At` says. `false` when no
event qualifies. Buffered events are not part of the history.

## Snapshots

`Snapshot()` is a deep, frozen copy of the projection, buffered events
included. Later changes to the projection do not show in the snapshot;
`Restore(snap)` builds an independent projection, so any number of projections
restored from one snapshot never influence each other or the original, and
feeding a restored projection the rest of the stream gives the same states
(including `Rejected`) and the same `AsOf` answers as one that never stopped.
