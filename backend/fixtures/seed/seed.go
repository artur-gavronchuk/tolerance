// Package seed loads the frontend mock data so a fresh stand looks like
// the v0 prototype. It runs with the migration role and only on an empty
// database; scores are inserted as completed human judgments so they go
// through the same columns the judge fills in slice 2.
package seed

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/db"
	"tolerance/internal/platform/idgen"
)

//go:embed *.json
var files embed.FS

var ErrNotEmpty = errors.New("seed: database is not empty")

type user struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
}

type agent struct {
	ID          string    `json:"id"`
	OwnerHandle string    `json:"owner_handle"`
	Name        string    `json:"name"`
	Model       string    `json:"model"`
	Bio         string    `json:"bio"`
	CreatedAt   time.Time `json:"created_at"`
}

type criterion struct {
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
	Description string `json:"description"`
}

type competition struct {
	ID         string      `json:"id"`
	Slug       string      `json:"slug"`
	Title      string      `json:"title"`
	Summary    string      `json:"summary"`
	Brief      string      `json:"brief"`
	Category   string      `json:"category"`
	Difficulty string      `json:"difficulty"`
	Status     string      `json:"status"`
	Points     int         `json:"points"`
	Deadline   time.Time   `json:"deadline"`
	Criteria   []criterion `json:"criteria"`
}

type score struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

type preview struct {
	Kind string `json:"kind"`
	Body string `json:"body"`
}

type submission struct {
	ID              string    `json:"id"`
	CompetitionSlug string    `json:"competition_slug"`
	AgentName       string    `json:"agent_name"`
	Artifact        string    `json:"artifact"`
	Summary         string    `json:"summary"`
	PreviewURL      *string   `json:"preview_url"`
	RepoURL         *string   `json:"repo_url"`
	Preview         *preview  `json:"preview"`
	SubmittedAt     time.Time `json:"submitted_at"`
	Scores          []score   `json:"scores"`
}

type badge struct {
	AgentName string   `json:"agent_name"`
	Codes     []string `json:"codes"`
}

func load[T any](name string) ([]T, error) {
	raw, err := files.ReadFile(name)
	if err != nil {
		return nil, err
	}
	var out []T
	return out, json.Unmarshal(raw, &out)
}

func Load(ctx context.Context, pool *db.Pool) error {
	users, err := load[user]("users.json")
	if err != nil {
		return err
	}
	agents, err := load[agent]("agents.json")
	if err != nil {
		return err
	}
	comps, err := load[competition]("competitions.json")
	if err != nil {
		return err
	}
	subs, err := load[submission]("submissions.json")
	if err != nil {
		return err
	}
	badges, err := load[badge]("badges.json")
	if err != nil {
		return err
	}
	return pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrNotEmpty
		}
		userByHandle := map[string]string{}
		for _, u := range users {
			userByHandle[u.Handle] = u.ID
			if _, err := tx.Exec(ctx, `INSERT INTO users (id, oidc_issuer, oidc_subject, email, handle, display_name) VALUES ($1,'seed',$2,$2,$3,$4)`,
				u.ID, u.Email, u.Handle, u.DisplayName); err != nil {
				return err
			}
		}
		agentByName := map[string]string{}
		for _, a := range agents {
			agentByName[a.Name] = a.ID
			if _, err := tx.Exec(ctx, `INSERT INTO agents (id, owner_user_id, name, model, bio, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
				a.ID, userByHandle[a.OwnerHandle], a.Name, a.Model, a.Bio, a.CreatedAt); err != nil {
				return err
			}
		}
		compBySlug := map[string]competition{}
		for _, c := range comps {
			compBySlug[c.Slug] = c
			criteria, err := json.Marshal(c.Criteria)
			if err != nil {
				return err
			}
			var closedAt *time.Time
			if c.Status == "closed" {
				d := c.Deadline
				closedAt = &d
			}
			if _, err := tx.Exec(ctx, `INSERT INTO competitions (id, slug, title, summary, brief, category, difficulty, status, points, deadline, criteria, created_by, published_at, closed_at)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
				c.ID, c.Slug, c.Title, c.Summary, c.Brief, c.Category, c.Difficulty, c.Status, c.Points, c.Deadline, criteria,
				userByHandle["nualimov"], c.Deadline.Add(-30*24*time.Hour), closedAt); err != nil {
				return err
			}
		}
		for _, s := range subs {
			c := compBySlug[s.CompetitionSlug]
			total := Total(c.Criteria, s.Scores)
			points := int(math.Round(float64(c.Points) * float64(total) / 100))
			scores := make([]map[string]any, 0, len(s.Scores))
			for _, sc := range s.Scores {
				scores = append(scores, map[string]any{"name": sc.Name, "score": sc.Score, "rationale": "Seeded from the prototype dataset."})
			}
			scoresJSON, err := json.Marshal(scores)
			if err != nil {
				return err
			}
			var previewKind, previewBody *string
			if s.Preview != nil {
				previewKind, previewBody = &s.Preview.Kind, &s.Preview.Body
			}
			jdg := idgen.New("jdg")
			// submissions.judgment_id references judgments(id) and
			// judgments.submission_id references submissions(id): neither
			// row can be inserted first with both foreign keys satisfied.
			// Break the cycle by inserting the submission with a NULL
			// judgment_id, then the judgment (now free to reference it),
			// then backfilling judgment_id.
			if _, err := tx.Exec(ctx, `INSERT INTO submissions (id, competition_id, agent_id, source, artifact, summary, preview_url, repo_url, preview_kind, preview_body, submitted_at, score_status, scores, total, points_awarded, judged_at)
				VALUES ($1,$2,$3,'manual',$4,$5,$6,$7,$8,$9,$10,'scored',$11,$12,$13,$10)`,
				s.ID, c.ID, agentByName[s.AgentName], s.Artifact, s.Summary, s.PreviewURL, s.RepoURL, previewKind, previewBody, s.SubmittedAt,
				scoresJSON, total, points); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO judgments (id, submission_id, kind, status, scores, total, overall, judge_user_id, created_at, completed_at)
				VALUES ($1,$2,'human','completed',$3,$4,'Seeded from the prototype dataset.',$5,$6,$6)`,
				jdg, s.ID, scoresJSON, total, userByHandle["nualimov"], s.SubmittedAt); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE submissions SET judgment_id = $2 WHERE id = $1`, s.ID, jdg); err != nil {
				return err
			}
		}
		for _, b := range badges {
			for _, code := range b.Codes {
				if _, err := tx.Exec(ctx, `INSERT INTO agent_badges (agent_id, code) VALUES ($1, $2)`, agentByName[b.AgentName], code); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Total implements invariant 6: round(Σ weight×score / 100). Slice 2's
// judging module reuses this function.
func Total(criteria []criterion, scores []score) int {
	weight := map[string]int{}
	for _, c := range criteria {
		weight[c.Name] = c.Weight
	}
	sum := 0.0
	for _, s := range scores {
		sum += float64(weight[s.Name]) * float64(s.Score)
	}
	return int(math.Round(sum / 100))
}
