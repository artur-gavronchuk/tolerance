// Package arena serves the platform's public read surface: the per-skill table
// of agents. It only reads — ratings are written by internal/qualifications.
package arena

import "time"

// Row is one line of a skill's public table. Model and harness travel with
// every row rather than living only on the agent's profile, because they are
// the two things a reader actually wants to compare.
type Row struct {
	Rank          int    `json:"rank"`
	AgentName     string `json:"agent_name"`
	VersionNumber int    `json:"version_number"`
	Model         string `json:"model"`
	Harness       string `json:"harness"`
	Rating        int    `json:"rating"`
	Uncertainty   int    `json:"uncertainty"`
	Access        int    `json:"access"`
	Tier          string `json:"tier"`
	Runs          int    `json:"runs"`
	// OnCurrentVersion is false when the rating was earned by a configuration the
	// owner has since replaced. The row still shows, and still says so.
	OnCurrentVersion bool      `json:"on_current_version"`
	ScoredAt         time.Time `json:"scored_at"`
}

// SkillSummary is a skill as the public arena shows it: enough to draw a tab and
// say whether the skill is open for runs. It carries no caller-specific state,
// which is what lets it be read without a session — unlike /skills, which also
// answers "can *I* start a run right now".
type SkillSummary struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Language    string `json:"language"`
	Description string `json:"description"`
	// PoolSize counts the tasks a run could be built from; Frozen is true when
	// that is too few to start one.
	PoolSize int  `json:"pool_size"`
	Frozen   bool `json:"frozen"`
}
