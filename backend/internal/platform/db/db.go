// Package db provides the connection pool and the only ways the rest of the
// codebase is allowed to open a transaction. Every write and every
// tenant-scoped read goes through one of these, which set the Postgres
// session variables Row Level Security policies key off; there is no code
// path that runs a query against a real table without either a scope or an
// explicit, named acknowledgement that the query is intentionally global.
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

// Tx runs fn inside a transaction scoped to organizationID: every tenant
// table's Row Level Security policy compares its organization_id column
// against this setting, so a query that forgets a WHERE clause still can't
// cross the tenant boundary. organizationID must not be empty. userID is
// set alongside it whenever the caller has one (which, past the identity
// middleware, is nearly always); pass "" only for service/job code that
// acts on behalf of the organization rather than a specific person.
func (p *Pool) Tx(ctx context.Context, organizationID, userID string, fn func(context.Context, pgx.Tx) error) error {
	if organizationID == "" {
		return fmt.Errorf("db: Tx called without an organization id")
	}
	return p.transact(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := setSessionVar(ctx, tx, "app.organization_id", organizationID); err != nil {
			return err
		}
		if userID != "" {
			if err := setSessionVar(ctx, tx, "app.user_id", userID); err != nil {
				return err
			}
		}
		return fn(ctx, tx)
	})
}

// SelfTx runs fn scoped only to userID, with no organization set. It exists
// for the handful of queries that are legitimately cross-tenant from a
// single user's point of view, such as "which organizations am I a member
// of" — the memberships table's Row Level Security carries a second,
// permissive policy for exactly this (see migration 00008). It grants no
// write access beyond what that policy allows; call sites should still read
// as an obviously narrow exception.
func (p *Pool) SelfTx(ctx context.Context, userID string, fn func(context.Context, pgx.Tx) error) error {
	if userID == "" {
		return fmt.Errorf("db: SelfTx called without a user id")
	}
	return p.transact(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := setSessionVar(ctx, tx, "app.user_id", userID); err != nil {
			return err
		}
		return fn(ctx, tx)
	})
}

// GlobalTx runs fn inside a transaction with no scope at all, for the
// handful of operations that are legitimately unscoped: creating an
// organization (and its first owner membership) before the organization
// exists to scope to, and resolving a user by their OIDC identity before
// their active organization is known. Every call site is expected to name
// why it needs this in a comment.
func (p *Pool) GlobalTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return p.transact(ctx, fn)
}

// SetScope sets the organization/user session variables on an
// already-open transaction, typically one begun with GlobalTx. It exists
// for the rare bootstrap sequence that creates a tenant-scoped row (an
// organization's first membership) inside the same transaction that
// created the tenant itself, where Tx's usual "scope known up front"
// requirement can't be satisfied yet when the transaction starts.
// organizationID and userID may each be "" to leave that variable unset.
func (p *Pool) SetScope(ctx context.Context, tx pgx.Tx, organizationID, userID string) error {
	if organizationID != "" {
		if err := setSessionVar(ctx, tx, "app.organization_id", organizationID); err != nil {
			return err
		}
	}
	if userID != "" {
		if err := setSessionVar(ctx, tx, "app.user_id", userID); err != nil {
			return err
		}
	}
	return nil
}

// setSessionVar sets a Postgres session variable for the current
// transaction only (SET LOCAL). SET cannot take a bind parameter, so the
// value is escaped via quote_literal; every value passed here is one of our
// own generated ids (idgen.New output), never raw user input, but it is
// escaped defensively in case that assumption is ever wrong.
func setSessionVar(ctx context.Context, tx pgx.Tx, name, value string) error {
	var quoted string
	if err := tx.QueryRow(ctx, "SELECT quote_literal($1)", value).Scan(&quoted); err != nil {
		return fmt.Errorf("quote %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL %s = %s", name, quoted)); err != nil {
		return fmt.Errorf("set %s: %w", name, err)
	}
	return nil
}

func (p *Pool) transact(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op if already committed
	if err := fn(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
