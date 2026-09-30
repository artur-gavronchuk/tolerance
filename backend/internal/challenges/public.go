package challenges

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/agents"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/proofs"
)

const summaryCols = `c.slug, c.title, c.summary, c.status, t.skill_slug, c.min_tier, c.opens_at, c.closes_at, c.prizes,
	(SELECT count(*) FROM challenge_entries e WHERE e.challenge_id = c.id)`

func scanSummary(row interface{ Scan(...any) error }, m *Summary) error {
	if err := row.Scan(&m.Slug, &m.Title, &m.Summary, &m.Status, &m.SkillSlug, &m.MinTier,
		&m.OpensAt, &m.ClosesAt, &m.Prizes, &m.Entrants); err != nil {
		return err
	}
	m.OpensAt, m.ClosesAt = m.OpensAt.UTC(), m.ClosesAt.UTC()
	return nil
}

// List is the public index. Open means entries are being accepted right now,
// Upcoming is opened but not yet started, Past is closed or published. A draft
// appears nowhere: until it opens it is nobody's business.
func (s *Service) List(ctx context.Context) (Lists, error) {
	out := Lists{Open: []Summary{}, Upcoming: []Summary{}, Past: []Summary{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+summaryCols+` FROM challenges c
			JOIN skill_tasks t ON t.slug = c.skill_task_slug
			WHERE c.status <> 'draft' ORDER BY c.closes_at DESC`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m Summary
			if err := scanSummary(rows, &m); err != nil {
				return err
			}
			switch {
			case m.Status == StatusOpen && m.OpensAt.After(nowUTC()):
				out.Upcoming = append(out.Upcoming, m)
			case m.Status == StatusOpen:
				out.Open = append(out.Open, m)
			default:
				out.Past = append(out.Past, m)
			}
		}
		return rows.Err()
	})
	return out, err
}

// Public is one challenge as anyone may see it. What it contains depends on the
// status, and the rules are the point: an open challenge must not leak the task
// its entrants are solving, a closed one shows places, and only a published one
// shows the task, the hidden test names and the diffs of consenting entrants.
func (s *Service) Public(ctx context.Context, slug string) (PublicView, error) {
	var v PublicView
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var id, taskSlug string
		var publishTests bool
		var taskMD string
		var hiddenTar []byte
		var language string
		err := tx.QueryRow(ctx, `SELECT `+summaryCols+`, c.id, c.skill_task_slug, c.publish_tests, t.task_md, t.hidden_tar, s.language
			FROM challenges c
			JOIN skill_tasks t ON t.slug = c.skill_task_slug
			JOIN skills s ON s.slug = t.skill_slug
			WHERE c.slug = $1 AND c.status <> 'draft'`, slug).
			Scan(&v.Slug, &v.Title, &v.Summary.Summary, &v.Status, &v.SkillSlug, &v.MinTier, &v.OpensAt, &v.ClosesAt,
				&v.Prizes, &v.Entrants, &id, &taskSlug, &publishTests, &taskMD, &hiddenTar, &language)
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.NotFound()
		}
		if err != nil {
			return err
		}
		v.OpensAt, v.ClosesAt = v.OpensAt.UTC(), v.ClosesAt.UTC()
		v.HiddenTests = []string{}
		v.Standings = []Standing{}
		if v.Status == StatusOpen {
			return nil
		}

		published := v.Status == StatusPublished
		if published {
			v.TaskMD = taskMD
			if publishTests {
				names, err := proofs.HiddenTestNames(language, hiddenTar)
				if err != nil {
					return err
				}
				v.HiddenTests = names
			}
		}
		// A place in a finished competition is a public fact: the agent's name
		// shows even if its owner has since left the public tables, and even if
		// the platform banned it. Privacy governs the rating tables, not history.
		rows, err := tx.Query(ctx, `SELECT a.name, coalesce(e.score, 0)::float8, coalesce(e.diff_lines, 0),
				coalesce(e.submitted_at, e.created_at), coalesce(e.rank, 0), e.consent_publish, coalesce(p.diff, '')
			FROM challenge_entries e
			JOIN agents a ON a.id = e.agent_id
			JOIN proofs p ON p.id = e.proof_id
			WHERE e.challenge_id = $1 ORDER BY e.rank, a.name`, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var st Standing
			var consent bool
			var diff string
			if err := rows.Scan(&st.AgentName, &st.Score, &st.DiffLines, &st.SubmittedAt, &st.Rank, &consent, &diff); err != nil {
				return err
			}
			st.SubmittedAt = st.SubmittedAt.UTC()
			if published && consent {
				st.Diff = diff
			}
			v.Standings = append(v.Standings, st)
		}
		return rows.Err()
	})
	return v, err
}

// nowUTC is a seam for nothing in particular; it keeps the comparison in List
// readable next to the UTC timestamps the scanner produces.
func nowUTC() time.Time { return time.Now().UTC() }

// PlacesFor is an agent's places in finished challenges, for its public profile.
// Only closed and published challenges count: a place does not exist until the
// deadline has passed.
func (s *Service) PlacesFor(ctx context.Context, agentID string) ([]agents.ChallengePlace, error) {
	out := []agents.ChallengePlace{}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT c.slug, c.title, e.rank,
				(SELECT count(*) FROM challenge_entries x WHERE x.challenge_id = c.id)
			FROM challenge_entries e JOIN challenges c ON c.id = e.challenge_id
			WHERE e.agent_id = $1 AND c.status IN ('closed', 'published') AND e.rank IS NOT NULL
			ORDER BY c.closes_at DESC`, agentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p agents.ChallengePlace
			if err := rows.Scan(&p.Slug, &p.Title, &p.Rank, &p.Of); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}
