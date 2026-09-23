package standings_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/dbtest"
	"tolerance/internal/standings"
)

// arrange: two users/agents, one closed competition (200 pts) where A scored 90 and B 80,
// one active competition (500 pts) where A scored 50; one finished match won by B.
func arrange(t *testing.T, d *dbtest.DB) {
	t.Helper()
	err := d.AdminPool.Tx(context.Background(), func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
		INSERT INTO users (id, oidc_issuer, oidc_subject, handle, display_name) VALUES
		  ('user_a','seed','a','alice','A'), ('user_b','seed','b','bob','B'), ('user_c','seed','c','carol','C');
		INSERT INTO agents (id, owner_user_id, name, model) VALUES
		  ('agent_a','user_a','Atlas','m'), ('agent_b','user_b','Sable','m'), ('agent_c','user_c','Nova','m');
		INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, criteria, created_by, published_at, closed_at) VALUES
		  ('comp_1','one','1','s','b','Full build','Easy','closed',200, now() - interval '1 day', '[{"name":"Functionality","weight":100,"description":"d"}]','user_a', now() - interval '2 days', now() - interval '1 day'),
		  ('comp_2','two','2','s','b','Bug fix','Easy','active',500, now() + interval '1 day', '[{"name":"Tests","weight":100,"description":"d"}]','user_a', now() - interval '2 days', NULL);
		INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, score_status, total, points_awarded, submitted_at) VALUES
		  ('sub_a1','comp_1','agent_a','manual','app','s','scored',90,180, now() - interval '3 days'),
		  ('sub_b1','comp_1','agent_b','manual','app','s','scored',80,160, now() - interval '3 days'),
		  ('sub_a2','comp_2','agent_a','manual','pr','s','scored',50,250, now() - interval '1 hour');
		INSERT INTO matches (id, competition_id, left_agent_id, right_agent_id, state, total_seconds, winner_agent_id, outcome, finished_at) VALUES
		  ('match_1','comp_2','agent_a','agent_b','finished',900,'agent_b','right', now());
		INSERT INTO agent_badges (agent_id, code) VALUES ('agent_a','first_entry');`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStandings_ViewAndRanks(t *testing.T) {
	d := dbtest.New(t)
	arrange(t, d)
	s := standings.NewService(d.AppPool)
	ctx := context.Background()

	a, err := s.ForAgent(ctx, "agent_a")
	if err != nil {
		t.Fatal(err)
	}
	if a.Points != 430 || a.Submissions != 2 || *a.Avg != 70 || a.CompetitionWins != 1 || a.MatchWins != 0 || a.Wins != 1 || *a.Rank != 1 {
		t.Fatalf("unexpected standing for A: %+v", a)
	}
	b, _ := s.ForAgent(ctx, "agent_b")
	if b.Points != 160 || b.MatchWins != 1 || b.Wins != 1 || *b.Rank != 2 {
		t.Fatalf("unexpected standing for B: %+v", b)
	}
	c, _ := s.ForAgent(ctx, "agent_c")
	if c.Rank != nil || c.Avg != nil || c.Points != 0 {
		t.Fatalf("agent without submissions must have nil rank/avg, got %+v", c)
	}
	board, _ := s.Leaderboard(ctx, 10)
	if len(board) != 2 || board[0].Agent != "Atlas" || board[1].Agent != "Sable" {
		t.Fatalf("leaderboard must list ranked agents only, in order: %+v", board)
	}
	badges, _ := s.BadgesForAgent(ctx, "agent_a")
	if len(badges) != 1 || badges[0].Label != "First entry" {
		t.Fatalf("badge must be resolved through the catalog: %+v", badges)
	}
}
