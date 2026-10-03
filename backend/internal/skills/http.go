package skills

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/identity"
	"tolerance/internal/platform/db"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/skillrating"
)

// Why a skill cannot be started right now (SkillView.BlockedReason); empty
// when it can.
const (
	BlockedNoAgent        = "no_agent"
	BlockedInProgress     = "in_progress"
	BlockedOffline        = "offline"
	BlockedNotOperational = "not_operational"
	BlockedNoVersion      = "no_version"
	BlockedDailyLimit     = "daily_limit"
	BlockedNoTasks        = "no_tasks"
)

// SkillView is one catalog entry with the caller's rating and whether a run
// can start right now. The fields are spelled out rather than embedding
// Skill, which would leak its internal image and run_cmd keys.
type SkillView struct {
	Slug          string                   `json:"slug"`
	Title         string                   `json:"title"`
	Language      string                   `json:"language"`
	Description   string                   `json:"description"`
	PoolSize      int                      `json:"pool_size"`
	Rating        *skillrating.SkillRating `json:"rating"`
	RunsToday     int                      `json:"runs_today"`
	CanStart      bool                     `json:"can_start"`
	BlockedReason string                   `json:"blocked_reason"`
}

type RatingsSource interface {
	RatingsFor(ctx context.Context, agentID string) ([]skillrating.SkillRating, error)
}

// RegisterOwnerRoutes mounts GET /skills: the catalog with the caller's
// rating and whether a run can start right now.
func RegisterOwnerRoutes(mux *http.ServeMux, pool *db.Pool, ratings RatingsSource, stageOf func(ctx context.Context, userID string) (agentID, stage string, hasVersion bool, err error)) {
	mux.HandleFunc("GET /api/v1/skills", func(w http.ResponseWriter, r *http.Request) {
		userID := identity.MustFromContext(r.Context()).UserID
		agentID, stage, hasVersion, err := stageOf(r.Context(), userID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var items []SkillView
		running := false
		err = pool.Tx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
			rows, err := tx.Query(ctx, `SELECT s.slug, s.title, s.language, s.description, (SELECT count(*) FROM skill_tasks t WHERE t.skill_slug = s.slug AND t.active),
				(SELECT count(*) FROM qualification_runs q WHERE q.skill_slug = s.slug AND q.agent_id = $1 AND q.created_at > now() - interval '24 hours')
				FROM skills s ORDER BY s.slug`, agentID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var v SkillView
				if err := rows.Scan(&v.Slug, &v.Title, &v.Language, &v.Description, &v.PoolSize, &v.RunsToday); err != nil {
					return err
				}
				items = append(items, v)
			}
			if err := rows.Err(); err != nil {
				return err
			}
			rows.Close()
			return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM qualification_runs WHERE agent_id = $1 AND status = 'running')`, agentID).Scan(&running)
		})
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var rs []skillrating.SkillRating
		if agentID != "" {
			if rs, err = ratings.RatingsFor(r.Context(), agentID); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
		}
		for i := range items {
			for j := range rs {
				if rs[j].SkillSlug == items[i].Slug {
					items[i].Rating = &rs[j]
				}
			}
			switch {
			case agentID == "":
				items[i].BlockedReason = BlockedNoAgent
			case running || stage == "checking":
				items[i].BlockedReason = BlockedInProgress
			case stage == "offline":
				items[i].BlockedReason = BlockedOffline
			case stage != "operational":
				items[i].BlockedReason = BlockedNotOperational
			case !hasVersion:
				items[i].BlockedReason = BlockedNoVersion
			case items[i].RunsToday >= 3:
				items[i].BlockedReason = BlockedDailyLimit
			case items[i].PoolSize < TasksPerRun:
				items[i].BlockedReason = BlockedNoTasks
			default:
				items[i].CanStart = true
			}
		}
		if items == nil {
			items = []SkillView{}
		}
		httpx.Respond(w, http.StatusOK, map[string]any{"items": items})
	})
}
