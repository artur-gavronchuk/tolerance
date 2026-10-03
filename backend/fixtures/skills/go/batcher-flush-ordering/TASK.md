# Write-behind batcher

Package `batcher` collects string items and hands them to a `Flush` callback
in batches. The visible tests pass, but the package misbehaves under the
contract below. Fix it without editing `*_test.go` files, `clock.go` or
`fakeclock.go`, and without adding dependencies. Hidden tests exercise this
contract, including with clocks that behave differently from `FakeClock`.

## Contract

- **Size trigger.** When the pending batch reaches `MaxItems` it is cut and
  delivered. `MaxItems <= 0` means no size limit.
- **Delay trigger.** The delay is measured from the *first* item of a batch:
  `MaxDelay` after that item arrived the batch is cut and delivered, however
  many items were added since. Later `Add`s never extend it. Once a batch is
  cut (for any reason) the next batch starts its own full delay with its own
  first item. `MaxDelay <= 0` means no time limit.
- **Stale timers.** A timer callback that belongs to an already-cut batch may
  still be invoked by a clock (a real clock cannot always cancel a timer that is
  already running). It must do nothing, in particular it must not cut the next
  batch early.
- **Ownership.** The slice given to `Flush` belongs to the callback. The
  batcher must never write to it afterwards, so a callback may keep it.
- **Ordering.** `Flush` calls are strictly serialized (never two at once) and
  happen in the order batches were cut, no matter which goroutine cut them.
- **No blocking on delivery.** `Add` must not wait for a `Flush` that another
  goroutine is delivering: the batch is queued and the goroutine already
  delivering will deliver it. `Flush` itself may call `Add`.
- **Empty batches** are never delivered.
- **Close.** `Close` cuts the pending batch, makes sure everything cut so far has
  been delivered, and only then returns, including batches another goroutine is
  still delivering. After `Close`, `Add` returns `ErrClosed`. Calling `Close`
  again is a no-op. (`Close` is not called from inside `Flush`.)
