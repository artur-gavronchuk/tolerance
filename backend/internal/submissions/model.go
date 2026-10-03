// Package submissions takes a person's result for a task (a zip of the edited repo, or a unified diff),
// turns it into a diff against the task's repo, and has a worker replay it against the hidden tests in the
// sandbox.
package submissions

import (
	"encoding/json"
	"time"
)

const (
	StatusQueued     = "queued"
	StatusRunning    = "running"
	StatusPassed     = "passed"
	StatusFailed     = "failed"
	StatusInfraError = "infra_error"

	// JobKind is the jobs.kind a submission's run is queued as.
	JobKind = "run_submission"

	MaxUploadBytes = 5 << 20
	maxDiffBytes   = 1 << 20
	maxMadeWith    = 100
	maxLogTail     = 32 << 10
)

type TestResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	// Optimize tasks: the case's score and, when invalid, why.
	Score  *float64 `json:"score,omitempty"`
	Reason string   `json:"reason,omitempty"`
}

// Submission is the API shape (see the daily contract).
type Submission struct {
	ID            string       `json:"id"`
	TaskSlug      string       `json:"task_slug"`
	Day           *string      `json:"day"`
	Status        string       `json:"status"`
	PassedTests   int          `json:"passed_tests"`
	TotalTests    int          `json:"total_tests"`
	Score         *float64     `json:"score"`
	FailureReason *string      `json:"failure_reason"`
	Tests         []TestResult `json:"tests"`
	LogTail       string       `json:"log_tail"`
	MadeWith      string       `json:"made_with"`
	CreatedAt     time.Time    `json:"created_at"`
	FinishedAt    *time.Time   `json:"finished_at"`
}

// RunPayload is the run_submission job payload.
type RunPayload struct {
	SubmissionID string `json:"submission_id"`
}

const cols = `s.id, s.task_slug, s.day::text, s.status, s.passed_tests, s.total_tests, s.score, s.failure_reason, s.tests, s.log_tail, s.made_with, s.created_at, s.finished_at`

type scanner interface{ Scan(...any) error }

func scan(row scanner) (Submission, error) {
	var s Submission
	var tests []byte
	if err := row.Scan(&s.ID, &s.TaskSlug, &s.Day, &s.Status, &s.PassedTests, &s.TotalTests, &s.Score, &s.FailureReason, &tests, &s.LogTail, &s.MadeWith, &s.CreatedAt, &s.FinishedAt); err != nil {
		return Submission{}, err
	}
	s.CreatedAt = s.CreatedAt.UTC()
	if s.FinishedAt != nil {
		t := s.FinishedAt.UTC()
		s.FinishedAt = &t
	}
	s.Tests = []TestResult{}
	if len(tests) > 0 {
		if err := json.Unmarshal(tests, &s.Tests); err != nil {
			return Submission{}, err
		}
	}
	return s, nil
}
