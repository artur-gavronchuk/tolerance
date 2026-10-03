package daily

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/tasks"
)

// HouseResult is how one house agent (a coding agent the platform runs itself, internal/house) did on a day,
// and how people compare. It is the "beat the house" summary shown next to the board.
type HouseResult struct {
	Handle   string `json:"handle"`
	Name     string `json:"name"`
	MadeWith string `json:"made_with"`
	// State: pending (not started), running (verdict not in), done, error (platform failure, no verdict) or
	// no_result (the agent produced nothing gradable).
	State       string   `json:"state"`
	Passed      bool     `json:"passed"` // every hidden test passed
	PassedTests int      `json:"passed_tests"`
	TotalTests  int      `json:"total_tests"`
	Score       *float64 `json:"score"`
	PeopleBeat  int      `json:"people_beat"` // people whose best attempt is strictly better
	PeopleTied  int      `json:"people_tied"` // people whose best attempt equals it
	// VsMe is the signed-in person's standing against this agent; nil when signed out or not yet ranked.
	VsMe *HouseVsMe `json:"vs_me"`
}

type HouseVsMe struct {
	Result string  `json:"result"` // beat | tied | behind
	Gap    float64 `json:"gap"`    // behind: how far the agent is ahead, in tests (bugfix) or score
}

// unranked is the rank of a min-task result with an invalid case: it loses to every valid result.
const unranked = -1e18

// rankSQL is the SQL expression (over submissions s, tasks t) a submission is ranked by, higher is better,
// the extra WHERE conditions a ranked submission must meet, and the floor a person's rank must exceed to
// count at all (people with nothing passed are not on the board).
func rankSQL(kind string, direction *string) (expr, where string, floor float64) {
	if kind != "optimize" {
		return `s.passed_tests::float8`, ``, 0
	}
	if direction != nil && *direction == "min" {
		return `(CASE WHEN s.cases_valid = t.cases THEN -s.score ELSE -1e18 END)`, ` AND s.score IS NOT NULL`, -1e17
	}
	return `s.score`, ` AND s.score IS NOT NULL`, -1e17
}

// houseResults builds the day's house summary; empty (not nil) when there are no house agents.
func (s *Service) houseResults(ctx context.Context, day string, task tasks.Summary, userID string) ([]HouseResult, error) {
	out := []HouseResult{}
	expr, where, floor := rankSQL(task.Kind, task.Direction)
	best := `
		SELECT DISTINCT ON (s.user_id) s.user_id, u.house, u.handle, u.house_name, s.made_with, s.status, s.passed_tests, s.total_tests, s.score, ` + expr + ` AS rank
		FROM submissions s JOIN users u ON u.id = s.user_id JOIN tasks t ON t.slug = s.task_slug
		WHERE s.day = $1::date AND s.status IN ('passed', 'failed') AND s.hidden_at IS NULL` + where + `
		ORDER BY s.user_id, rank DESC, s.created_at ASC`

	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		type house struct{ handle, name string }
		var houses []house
		rows, err := tx.Query(ctx, `SELECT handle, house_name FROM users WHERE house AND banned_at IS NULL ORDER BY handle`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var h house
			if err := rows.Scan(&h.handle, &h.name); err != nil {
				rows.Close()
				return err
			}
			houses = append(houses, h)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(houses) == 0 {
			return nil
		}

		// Latest submission of any status per house agent, for the pending / running / error states.
		latest := map[string]string{}
		lrows, err := tx.Query(ctx, `
			SELECT DISTINCT ON (s.user_id) u.handle, s.status FROM submissions s JOIN users u ON u.id = s.user_id
			WHERE u.house AND s.day = $1::date AND s.hidden_at IS NULL ORDER BY s.user_id, s.created_at DESC`, day)
		if err != nil {
			return err
		}
		for lrows.Next() {
			var h, st string
			if err := lrows.Scan(&h, &st); err != nil {
				lrows.Close()
				return err
			}
			latest[h] = st
		}
		lrows.Close()
		if err := lrows.Err(); err != nil {
			return err
		}

		type res struct {
			HouseResult
			rank float64
		}
		done := map[string]res{}
		brows, err := tx.Query(ctx, `
			WITH best AS (`+best+`)
			SELECT h.handle, h.made_with, h.status, h.passed_tests, h.total_tests, h.score, h.rank,
			  (SELECT count(*) FROM best p WHERE NOT p.house AND p.rank > $2 AND p.rank > h.rank),
			  (SELECT count(*) FROM best p WHERE NOT p.house AND p.rank > $2 AND p.rank = h.rank)
			FROM best h WHERE h.house`, day, floor)
		if err != nil {
			return err
		}
		for brows.Next() {
			var r res
			var status string
			if err := brows.Scan(&r.Handle, &r.MadeWith, &status, &r.PassedTests, &r.TotalTests, &r.Score, &r.rank, &r.PeopleBeat, &r.PeopleTied); err != nil {
				brows.Close()
				return err
			}
			r.Passed = status == "passed"
			done[r.Handle] = r
		}
		brows.Close()
		if err := brows.Err(); err != nil {
			return err
		}

		var myRank *float64
		if userID != "" {
			var r float64
			err := tx.QueryRow(ctx, `WITH best AS (`+best+`) SELECT rank FROM best WHERE user_id = $3 AND rank > $2`, day, floor, userID).Scan(&r)
			if err == nil {
				myRank = &r
			} else if err != pgx.ErrNoRows {
				return err
			}
		}

		configured := map[string]bool{}
		for _, h := range s.HouseHandles {
			configured[h] = true
		}
		for _, h := range houses {
			var hr HouseResult
			if r, ok := done[h.handle]; ok {
				hr = r.HouseResult
				hr.State = "done"
				if myRank != nil {
					switch {
					case *myRank > r.rank:
						hr.VsMe = &HouseVsMe{Result: "beat"}
					case *myRank == r.rank:
						hr.VsMe = &HouseVsMe{Result: "tied"}
					default:
						hr.VsMe = &HouseVsMe{Result: "behind", Gap: r.rank - *myRank}
					}
				}
			} else {
				switch st := latest[h.handle]; st {
				case "queued", "running":
					hr.State = "running"
				case "infra_error":
					hr.State = "error"
				case "passed", "failed":
					hr.State = "no_result" // finished but unranked (no score on an optimize task)
				default:
					if day != Today() || !configured[h.handle] {
						continue // a past day without a run, or an agent no longer configured
					}
					hr.State = "pending"
				}
			}
			hr.Handle, hr.Name = h.handle, h.name
			out = append(out, hr)
		}
		return nil
	})
	// Configured order first (the order of the config file), then any other by handle.
	pos := map[string]int{}
	for i, h := range s.HouseHandles {
		pos[h] = i + 1
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := pos[out[i].Handle], pos[out[j].Handle]
		if a == 0 || b == 0 {
			if a != b {
				return a != 0
			}
			return out[i].Handle < out[j].Handle
		}
		return a < b
	})
	return out, err
}
