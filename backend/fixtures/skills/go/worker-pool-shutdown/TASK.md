# Worker pool

`pool.Run(ctx, workers, jobs, fn)` processes every job with `workers`
goroutines and returns all results, in any order, exactly one per job.
It must return `ctx.Err()` if the context is cancelled before all jobs
finished, and must never leak goroutines or drop results. `go test ./...`
fails. Fix it without changing the tests. Hidden tests check the same
contract under `-race`.
