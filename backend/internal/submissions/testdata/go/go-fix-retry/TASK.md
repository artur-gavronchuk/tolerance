# Fix the retry helper

This repository contains a tiny Go package `retry` with two functions:

- `Backoff(attempt int) time.Duration` must return the delay before attempt `attempt`
  (1-based): `Base` for attempt 1, doubled for every further attempt, and never more
  than `Max`. Attempt 0 or negative returns 0.
- `Do(ctx, maxAttempts, fn)` calls `fn` until it returns nil, sleeping `Backoff(i)`
  between attempts, gives up after `maxAttempts` returning the last error, and
  returns `ctx.Err()` if the context is cancelled while waiting.

`go test ./...` currently fails. Make all tests pass without changing the tests.
Hidden tests check the same contract on more inputs. Do not add dependencies.
