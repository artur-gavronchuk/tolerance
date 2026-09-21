// Package submissions turns a finished attempt into a submission: the
// participant's declared result, frozen together with the agent
// configuration it ran with, queued for automated checks, and later
// scored from the evidence those checks produce.
package submissions

import (
	"encoding/json"
	"time"
)

const (
	StatusPending      = "pending"
	StatusJudging      = "judging"
	StatusScored       = "scored"
	StatusUnverifiable = "unverifiable"
	StatusFailed       = "failed"
)

// Cost is what the owner's connector reports; the platform cannot verify it.
type Cost struct {
	USD    float64 `json:"usd"`
	Source string  `json:"source"`
}

// Input is the participant's result, as written to arena-result.json.
type Input struct {
	Summary    string `json:"summary"`
	PreviewURL string `json:"preview_url"`
	RepoURL    string `json:"repo_url"`
	CommitSHA  string `json:"commit_sha"`
	Notes      string `json:"notes"`
	Cost       *Cost  `json:"cost"`
}

// ScoreEntry is one criterion's outcome. Reason is a machine code for why a
// criterion is not rated (judge_disabled, source_not_available, judge_failed).
type ScoreEntry struct {
	Name      string `json:"name"`
	Source    string `json:"source"`
	Weight    int    `json:"weight"`
	Score     *int   `json:"score"`
	Status    string `json:"status"` // rated | not_rated
	Reason    string `json:"reason,omitempty"`
	Rationale string `json:"rationale"`
}

type AttemptInfo struct {
	ID              *string    `json:"id"`
	Kind            string     `json:"kind"`
	No              int        `json:"no"`
	StartedAt       *time.Time `json:"started_at"`
	DurationSeconds *int       `json:"duration_seconds"`
}

type CheckRunInfo struct {
	ID              string     `json:"id"`
	Status          string     `json:"status"`
	Suite           string     `json:"suite"`
	SuiteVersion    string     `json:"suite_version"`
	FunctionalScore *int       `json:"functional_score"`
	FinishedAt      *time.Time `json:"finished_at"`
}

type Preview struct {
	Kind string `json:"kind"`
	Body string `json:"body"`
}

// View is the public shape of a submission.
type View struct {
	ID              string          `json:"id"`
	CompetitionID   string          `json:"competition_id"`
	CompetitionSlug string          `json:"competition_slug"`
	Agent           string          `json:"agent"`
	Author          string          `json:"author"`
	Source          string          `json:"source"`
	MatchID         *string         `json:"match_id"`
	SubmittedAt     time.Time       `json:"submitted_at"`
	Artifact        string          `json:"artifact"`
	Summary         string          `json:"summary"`
	PreviewURL      *string         `json:"preview_url"`
	RepoURL         *string         `json:"repo_url"`
	Preview         *Preview        `json:"preview"`
	ScoreStatus     string          `json:"score_status"`
	Total           *int            `json:"total"`
	Scores          []ScoreEntry    `json:"scores"`
	PointsAwarded   *int            `json:"points_awarded"`
	JudgedAt        *time.Time      `json:"judged_at"`
	Rank            *int            `json:"rank"`
	RankOf          *int            `json:"rank_of"`
	Attempt         AttemptInfo     `json:"attempt"`
	AgentSnapshot   json.RawMessage `json:"agent_snapshot"`
	CommitSHA       *string         `json:"commit_sha"`
	CommitLink      string          `json:"commit_link"`
	Verification    string          `json:"verification"`
	Notes           *string         `json:"notes"`
	ReportedCost    json.RawMessage `json:"reported_cost"`
	UnscoredReason  *string         `json:"unscored_reason"`
	CheckRun        *CheckRunInfo   `json:"check_run"`
	Limitations     []string        `json:"limitations"`
}

// Submitted is the response to creating a submission.
type Submitted struct {
	View
	ResultURL string `json:"result_url"`
}
