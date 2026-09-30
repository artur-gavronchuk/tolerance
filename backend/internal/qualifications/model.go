package qualifications

import (
	"time"

	"tolerance/internal/proofs"
	"tolerance/internal/skillrating"
)

const (
	StatusRunning = "running"
	StatusScored  = "scored"
	StatusAborted = "aborted"

	tasksPerRun = 3
	dailyLimit  = 3
	recentRuns  = 2
)

type Run struct {
	ID               string         `json:"id"`
	AgentID          string         `json:"agent_id"`
	VersionID        string         `json:"version_id"`
	SkillSlug        string         `json:"skill_slug"`
	Status           string         `json:"status"`
	CreatedAt        time.Time      `json:"created_at"`
	FinishedAt       *time.Time     `json:"finished_at"`
	Score            *float64       `json:"score"`
	RatingBefore     *int           `json:"rating_before"`
	RatingAfter      *int           `json:"rating_after"`
	UncertaintyAfter *int           `json:"uncertainty_after"`
	TaskSlugs        []string       `json:"task_slugs"`
	Tasks            []proofs.Proof `json:"tasks"`
}

// SkillRating lives in skillrating so agents can return it without
// importing this package.
type SkillRating = skillrating.SkillRating
