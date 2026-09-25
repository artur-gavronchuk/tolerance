package proofs

import (
	"time"

	"tolerance/internal/proofs/sandbox"
)

const (
	StatusQueued         = "queued"
	StatusClaimed        = "claimed"
	StatusRunningAgent   = "running_agent"
	StatusDiffSubmitted  = "diff_submitted"
	StatusRunningSandbox = "running_sandbox"
	StatusPassed         = "passed"
	StatusFailed         = "failed"
	StatusInfraError     = "infra_error"
	StatusExpired        = "expired"

	// KindProof is a plain go-fix-retry-style proof task, verified by hidden tests in the sandbox.
	KindProof = "proof"
	// KindGameBot is a proof task whose diff becomes a bot version instead of running hidden tests.
	KindGameBot = "game_bot"

	maxDiffBytes    = 256 << 10
	maxLogTailBytes = 32 << 10
	dailyLimit      = 10
	claimTimeout    = 5 * time.Minute

	// maxResultBodyBytes bounds the result request: a diff at the 256 KiB
	// limit plus JSON escaping and a 32 KiB log tail fit well inside it.
	maxResultBodyBytes = 1 << 20
)

var openStatuses = []string{StatusQueued, StatusClaimed, StatusRunningAgent, StatusDiffSubmitted, StatusRunningSandbox}

type Task struct {
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Language        string `json:"language"`
	Kind            string `json:"-"`
	Image           string `json:"-"`
	RunCmd          string `json:"-"`
	AgentTimeoutS   int    `json:"agent_timeout_s"`
	SandboxTimeoutS int    `json:"sandbox_timeout_s"`
	VisibleTests    int    `json:"visible_tests"`
	HiddenTests     int    `json:"hidden_tests"`
	TaskMD          string `json:"task_md"`
	RepoTar         []byte `json:"-"`
	HiddenTar       []byte `json:"-"`
	RepoSHA256      string `json:"repo_sha256"`
}

type TestResult = sandbox.TestResult

type SandboxResult struct {
	Tests    []TestResult `json:"tests"`
	ExitCode int          `json:"exit_code"`
	Output   string       `json:"output"`
	TimedOut bool         `json:"timed_out"`
}

type Proof struct {
	ID              string         `json:"id"`
	AgentID         string         `json:"agent_id"`
	TaskSlug        string         `json:"task_slug"`
	Status          string         `json:"status"`
	CreatedAt       time.Time      `json:"created_at"`
	ClaimedAt       *time.Time     `json:"claimed_at"`
	DiffSubmittedAt *time.Time     `json:"diff_submitted_at"`
	FinishedAt      *time.Time     `json:"finished_at"`
	Diff            string         `json:"diff"`
	AgentLogTail    string         `json:"agent_log_tail"`
	AgentDurationMS *int           `json:"agent_duration_ms"`
	AgentExitCode   *int           `json:"agent_exit_code"`
	SandboxResult   *SandboxResult `json:"sandbox_result"`
	FailureReason   string         `json:"failure_reason"`
	Kind            string         `json:"kind"`
}
