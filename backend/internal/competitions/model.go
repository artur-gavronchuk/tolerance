package competitions

import (
	"encoding/json"
	"time"
)

// Criterion sources: "checks" is measured by the automated check suite,
// "llm" is rated qualitatively (or reported as not rated).
const (
	SourceChecks = "checks"
	SourceLLM    = "llm"
)

type Criterion struct {
	Name        string `json:"name"`
	Weight      int    `json:"weight"`
	Description string `json:"description"`
	Source      string `json:"source,omitempty"`
}

const (
	StatusDraft  = "draft"
	StatusActive = "active"
	StatusClosed = "closed"
)

type Competition struct {
	ID                   string
	Slug                 string
	Title                string
	Summary              string
	Brief                string
	Category             string
	Difficulty           string
	Status               string
	Points               int
	Deadline             time.Time
	MatchDurationSeconds int
	Criteria             []Criterion
	CreatedBy            string
	CreatedAt            time.Time
	PublishedAt          *time.Time
	ClosedAt             *time.Time
	Version              int
	Participants         int
	ScoredCount          int
	Task                 json.RawMessage
	CheckSuite           string
}

// PublicView is the shape the frontend's Competition type maps onto.
type PublicView struct {
	ID                   string          `json:"id"`
	Slug                 string          `json:"slug"`
	Title                string          `json:"title"`
	Summary              string          `json:"summary"`
	Brief                string          `json:"brief"`
	Category             string          `json:"category"`
	Difficulty           string          `json:"difficulty"`
	Status               string          `json:"status"` // active | past
	Points               int             `json:"points"`
	Deadline             time.Time       `json:"deadline"`
	Participants         int             `json:"participants"`
	ScoredCount          int             `json:"scored_count"`
	MatchDurationSeconds int             `json:"match_duration_seconds"`
	Criteria             []Criterion     `json:"criteria"`
	Task                 json.RawMessage `json:"task"`
	CheckSuite           string          `json:"check_suite,omitempty"`
}

func (c Competition) Public() PublicView {
	status := "active"
	if c.Status == StatusClosed {
		status = "past"
	}
	return PublicView{ID: c.ID, Slug: c.Slug, Title: c.Title, Summary: c.Summary, Brief: c.Brief, Category: c.Category,
		Difficulty: c.Difficulty, Status: status, Points: c.Points, Deadline: c.Deadline.UTC(), Participants: c.Participants,
		ScoredCount: c.ScoredCount, MatchDurationSeconds: c.MatchDurationSeconds, Criteria: c.Criteria,
		Task: c.Task, CheckSuite: c.CheckSuite}
}

// AdminView adds lifecycle fields and the raw status.
type AdminView struct {
	PublicView
	RawStatus   string     `json:"raw_status"`
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	PublishedAt *time.Time `json:"published_at"`
	ClosedAt    *time.Time `json:"closed_at"`
	Version     int        `json:"version"`
}

func (c Competition) Admin() AdminView {
	return AdminView{PublicView: c.Public(), RawStatus: c.Status, CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt.UTC(),
		PublishedAt: c.PublishedAt, ClosedAt: c.ClosedAt, Version: c.Version}
}

type Input struct {
	Slug                 string          `json:"slug"`
	Title                string          `json:"title"`
	Summary              string          `json:"summary"`
	Brief                string          `json:"brief"`
	Category             string          `json:"category"`
	Difficulty           string          `json:"difficulty"`
	Points               int             `json:"points"`
	Deadline             time.Time       `json:"deadline"`
	MatchDurationSeconds int             `json:"match_duration_seconds"`
	Criteria             []Criterion     `json:"criteria"`
	Task                 json.RawMessage `json:"task,omitempty"`
	CheckSuite           string          `json:"check_suite,omitempty"`
}
