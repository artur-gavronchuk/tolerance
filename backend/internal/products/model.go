// Package products runs weekly product tasks: a task with a deadline and a few attempts. A person's agent
// builds the thing, the person uploads a zip, the sandbox scores it with the task's I/O scenarios, and after
// the deadline all entries are published and opened for voting.
package products

import (
	"encoding/json"
	"time"
)

const (
	StatusQueued     = "queued"
	StatusRunning    = "running"
	StatusDone       = "done"
	StatusInfraError = "infra_error"

	// JobKind is the jobs.kind an entry's scoring run is queued as.
	JobKind = "run_product"

	// AttemptsPerTask is how many uploads a person gets per task; the best one counts.
	AttemptsPerTask = 3

	MaxUploadBytes = 5 << 20
	maxMadeWith    = 100
	maxLogTail     = 8 << 10
	stuckAfter     = 15 * time.Minute

	KindCLI  = "cli"  // a command-line tool scored by I/O scenarios in the sandbox
	KindSite = "site" // a static site judged by votes only

	PhaseOpen   = "open"   // before the deadline: uploads allowed, entries hidden
	PhaseVoting = "voting" // after the deadline: uploads closed, entries public, voting open
)

// Failure reasons stored in product_entries.failure_reason.
const (
	ReasonNoEntrypoint = "no_entrypoint"
	ReasonNoResults    = "no_results"
	ReasonTimeout      = "timeout"
	reasonStuck        = "stuck"
)

// Scenario is one I/O check: the tool is started with Args, fed Stdin, and must print Stdout and exit with
// ExitCode.
type Scenario struct {
	Name     string   `json:"name"`
	Args     []string `json:"args"`
	Stdin    string   `json:"stdin"`
	Stdout   string   `json:"stdout"`
	ExitCode int      `json:"exit_code"`

	// Site kind: browser steps (see runner_site.py), optionally at a viewport width.
	Viewport int               `json:"viewport,omitempty"`
	Steps    []json.RawMessage `json:"steps,omitempty"`
}

type ScenarioResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
}

// Task is the API shape of a product task.
type Task struct {
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Summary       string    `json:"summary"`
	Kind          string    `json:"kind"`
	Phase         string    `json:"phase"`
	OpensAt       time.Time `json:"opens_at"`
	Deadline      time.Time `json:"deadline"`
	ScenarioCount int       `json:"scenario_count"`
	Attempts      int       `json:"attempts"`
	EntryCount    int       `json:"entry_count"`
	TaskMD        string    `json:"task_md,omitempty"`
}

// Entry is one scored upload. Handle is only filled for published entries.
type Entry struct {
	ID            string           `json:"id"`
	TaskSlug      string           `json:"task_slug"`
	Handle        string           `json:"handle,omitempty"`
	Status        string           `json:"status"`
	Passed        int              `json:"passed"`
	Total         int              `json:"total"`
	FailureReason *string          `json:"failure_reason"`
	Results       []ScenarioResult `json:"results"`
	LogTail       string           `json:"log_tail,omitempty"`
	MadeWith      string           `json:"made_with"`
	Votes         int              `json:"votes"`
	Voted         bool             `json:"voted"`
	Mine          bool             `json:"mine"`
	CreatedAt     time.Time        `json:"created_at"`
	FinishedAt    *time.Time       `json:"finished_at"`
}

// RunPayload is the run_product job payload.
type RunPayload struct {
	EntryID string `json:"entry_id"`
}

type scanner interface{ Scan(...any) error }

// entryCols expects the alias e for product_entries, u for users, $1 = the viewer's user id.
const entryCols = `e.id, e.task_slug, u.handle, e.status, e.passed, e.total, e.failure_reason, e.results, e.log_tail, e.made_with,
	(SELECT count(*) FROM product_votes v WHERE v.entry_id = e.id),
	EXISTS (SELECT 1 FROM product_votes v WHERE v.entry_id = e.id AND v.user_id = $1), e.user_id = $1, e.created_at, e.finished_at`

func scanEntry(row scanner) (Entry, error) {
	var e Entry
	var results []byte
	if err := row.Scan(&e.ID, &e.TaskSlug, &e.Handle, &e.Status, &e.Passed, &e.Total, &e.FailureReason, &results, &e.LogTail,
		&e.MadeWith, &e.Votes, &e.Voted, &e.Mine, &e.CreatedAt, &e.FinishedAt); err != nil {
		return Entry{}, err
	}
	e.CreatedAt = e.CreatedAt.UTC()
	if e.FinishedAt != nil {
		t := e.FinishedAt.UTC()
		e.FinishedAt = &t
	}
	e.Results = []ScenarioResult{}
	if len(results) > 0 {
		if err := json.Unmarshal(results, &e.Results); err != nil {
			return Entry{}, err
		}
	}
	return e, nil
}
