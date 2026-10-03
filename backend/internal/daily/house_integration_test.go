package daily_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/daily"
	"tolerance/internal/platform/dbtest"
	"tolerance/internal/tasks"
)

// House agents share the day's board but never take a place, and never count as people anywhere.
func TestHouseRowsTakeNoPlaceAndAreNotPeople(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	ts, err := tasks.LoadFlat(filepath.Join("..", "..", "fixtures", "proofs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.Sync(ctx, d.AdminPool, ts); err != nil {
		t.Fatal(err)
	}
	svc := daily.NewService(d.AppPool)
	svc.HouseHandles = []string{"opus"}
	slug, err := svc.TaskFor(ctx, daily.Today())
	if err != nil {
		t.Fatal(err)
	}
	day := daily.Today()

	add := func(id, handle string, house bool, passed int) {
		t.Helper()
		if err := d.AdminPool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `INSERT INTO users (id, email, handle, house, house_name) VALUES ($1, $2, $3, $4, $5)`,
				id, handle+"@example.com", handle, house, "Claude Code · "+handle); err != nil {
				return err
			}
			status := "failed"
			if passed == 5 {
				status = "passed"
			}
			_, err := tx.Exec(ctx, `INSERT INTO submissions (id, user_id, task_slug, day, diff, status, passed_tests, total_tests, finished_at)
				VALUES ($1, $2, $3, $4::date, 'x', $5, $6, 5, now())`, "sub_"+id, id, slug, day, status, passed)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	add("u_best", "best", false, 5)
	add("u_house", "opus", true, 4)
	add("u_mid", "mid", false, 3)

	board, err := svc.Leaderboard(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(board) != 3 {
		t.Fatalf("board has %d rows, want 3: %+v", len(board), board)
	}
	wantPlace := []int{1, 0, 2}
	for i, r := range board {
		if r.Place != wantPlace[i] || r.House != (r.Handle == "opus") {
			t.Fatalf("row %d = %+v, want place %d", i, r, wantPlace[i])
		}
	}

	over, err := svc.Overall(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range over {
		if r.Handle == "opus" {
			t.Fatal("house agent is on the overall leaderboard")
		}
	}
	if len(over) != 2 {
		t.Fatalf("overall has %d people, want 2", len(over))
	}
	st, err := svc.Stats(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	if st.Participants != 2 || st.Solvers != 1 {
		t.Fatalf("stats count the house: %+v", st)
	}

	// The strip: one person beats the house agent; the signed-in one trails it by 1 test.
	g, err := svc.Get(ctx, day, "u_mid", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.House) != 1 {
		t.Fatalf("house summary = %+v", g.House)
	}
	h := g.House[0]
	if h.State != "done" || h.PassedTests != 4 || h.PeopleBeat != 1 || h.PeopleTied != 0 {
		t.Fatalf("house result = %+v", h)
	}
	if h.VsMe == nil || h.VsMe.Result != "behind" || h.VsMe.Gap != 1 {
		t.Fatalf("vs_me = %+v", h.VsMe)
	}
}
