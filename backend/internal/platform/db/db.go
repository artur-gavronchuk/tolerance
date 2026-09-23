// Package db provides the connection pool and the single way the rest of
// the codebase opens a transaction. Arena data is public and not
// multi-tenant, so there is no per-request scope to set; Tx exists so
// every write goes through one place (begin, run, commit-or-rollback).
package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Pool struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Pool{pool: pool}, nil
}

func (p *Pool) Close() { p.pool.Close() }

// Raw exposes the underlying pool for the few places that need a dedicated
// connection (LISTEN in slice 3) or a plain query outside a transaction.
func (p *Pool) Raw() *pgxpool.Pool { return p.pool }

// Tx runs fn in a transaction, committing if it returns nil and rolling
// back otherwise. Errors from fn are returned unwrapped so callers can
// errors.As them into *httpx.Problem.
func (p *Pool) Tx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
