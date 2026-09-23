// Package idempotency makes a POST command safe to retry: the same
// Idempotency-Key with the same body replays the first response instead of
// running the mutation again, and the same key with a different body is
// rejected outright.
//
// Known limitation: the mutation and the recording of its result are two
// separate transactions (the mutation's own, then a follow-up INSERT). A
// crash in the narrow window between them leaves no record, so a retry
// after that exact crash would run the mutation again. Two genuinely
// concurrent requests with the same key before either has recorded a
// result will likewise both run the mutation; the ON CONFLICT DO NOTHING
// below only prevents a corrupted second row, not a second side effect.
// This is an accepted, documented gap for slice 1's commands (sequential
// retries of the same client), not a claim of exactly-once semantics.
//
// The stored body round-trips through a jsonb column, so a replayed
// response is reformatted (Postgres's own jsonb-to-text spacing) rather
// than byte-identical to the original. The JSON value is unchanged; only a
// byte-for-byte comparison of the two responses would notice.
package idempotency

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
)

// Mutation performs the actual write and reports the HTTP status to record
// it under. It receives the request itself, not just its context, so it
// can read path values (r.PathValue) the way any other handler would;
// returning an error aborts before anything is recorded.
type Mutation func(r *http.Request, actor identity.Actor, raw []byte) (result any, status int, err error)

// Command wraps fn with idempotency-key handling and returns a ready-to-use
// handler. endpoint should be a stable string identifying this route (e.g.
// "POST /campaigns"); it is part of the dedupe key, so retries of a
// different route with the same header value never collide. Records are
// keyed by the calling actor (user or agent), never by tenant — Arena has
// no tenants.
func Command(pool *db.Pool, endpoint string, fn Mutation) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor := identity.MustFromContext(r.Context())
		key := r.Header.Get("Idempotency-Key")
		if len(key) < 8 || len(key) > 128 {
			httpx.WriteError(w, r, httpx.New(http.StatusUnprocessableEntity, "invalid_key", "Idempotency-Key must be 8-128 characters"))
			return
		}
		raw, err := httpx.ReadBody(w, r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		digest := idgen.Digest(raw)

		status, body, found, err := lookup(r.Context(), pool, actor, endpoint, key, digest)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if found {
			respondRaw(w, status, body)
			return
		}

		result, status, err := fn(r, actor, raw)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		body, err = json.Marshal(result)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if err := save(r.Context(), pool, actor, endpoint, key, digest, status, body); err != nil {
			// The mutation already committed; failing to persist the replay
			// record only weakens a future retry, so this does not turn an
			// otherwise-successful request into a failure for the client.
			log.Printf("idempotency: failed to persist record for %s %s: %v", endpoint, key, err)
		}
		respondRaw(w, status, body)
	}
}

func lookup(ctx context.Context, pool *db.Pool, actor identity.Actor, endpoint, key, digest string) (status int, body []byte, found bool, err error) {
	txErr := pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var storedDigest string
		scanErr := tx.QueryRow(ctx, `
			SELECT payload_digest, status, body FROM idempotency_records
			WHERE actor_id = $1 AND endpoint = $2 AND key = $3`,
			actor.ID, endpoint, key).Scan(&storedDigest, &status, &body)
		if scanErr == pgx.ErrNoRows {
			return nil
		}
		if scanErr != nil {
			return scanErr
		}
		if storedDigest != digest {
			return httpx.New(http.StatusConflict, "idempotency_conflict", "This key was already used with a different body")
		}
		found = true
		return nil
	})
	if txErr != nil {
		return 0, nil, false, txErr
	}
	return status, body, found, nil
}

func save(ctx context.Context, pool *db.Pool, actor identity.Actor, endpoint, key, digest string, status int, body []byte) error {
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO idempotency_records (actor_id, endpoint, key, payload_digest, status, body, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, now() + interval '90 days')
			ON CONFLICT (actor_id, endpoint, key) DO NOTHING`,
			actor.ID, endpoint, key, digest, status, body)
		return err
	})
}

func respondRaw(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
