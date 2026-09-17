package main_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/fixtures/seed"
	"tolerance/internal/platform/dbtest"
)

func TestSeed_LoadsMockAndRefusesTwice(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	if err := seed.Load(ctx, d.AdminPool); err != nil {
		t.Fatal(err)
	}
	var comps, agentsN, subs, badges int
	var atlasTotal int
	err := d.AppPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM competitions`).Scan(&comps); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM agents`).Scan(&agentsN); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE score_status = 'scored'`).Scan(&subs); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM agent_badges`).Scan(&badges); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT total FROM submissions WHERE id = 'sub_seed_wp-atlas'`).Scan(&atlasTotal)
	})
	if err != nil {
		t.Fatal(err)
	}
	if comps != 6 || agentsN != 6 || subs != 8 || badges != 6 || atlasTotal != 92 {
		t.Fatalf("comps=%d agents=%d subs=%d badges=%d atlasTotal=%d", comps, agentsN, subs, badges, atlasTotal)
	}
	if err := seed.Load(ctx, d.AdminPool); err != seed.ErrNotEmpty {
		t.Fatalf("second load must refuse: %v", err)
	}
}
