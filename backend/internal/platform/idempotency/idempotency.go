// Package idempotency makes a POST command safe to retry: the same
// Idempotency-Key with the same body replays the first response instead of
// running the mutation again, and the same key with a different body is
// rejected outright.
//
// Rewritten in Task 7 of docs/plans/arena-slice-1-foundation.md to key
// records by identity.Actor instead of an organization; this placeholder
// only keeps the rest of the platform package compiling until then.
package idempotency
