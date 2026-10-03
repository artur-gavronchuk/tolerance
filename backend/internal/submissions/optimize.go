package submissions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/platform/sanitize"
	"tolerance/internal/sandbox"
	"tolerance/internal/tasks"
)

// runOptimize handles an optimize task: the diff is applied to the repo as for any task (no test-file or
// harness rules: there are no tests), the hidden cases and score.py are unpacked next to it, never into it.
func (w *Worker) runOptimize(ctx context.Context, id string, in runInput) error {
	names, err := tasks.HiddenCaseNames(in.hiddenTar)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("submissions: task %s has no cases", in.slug)
	}
	dir, err := os.MkdirTemp(w.workDir, "sub-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	hiddenDir, err := os.MkdirTemp(w.workDir, "hid-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(hiddenDir)
	if err := tasks.Untar(in.repoTar, dir); err != nil {
		return err
	}
	if reason := applyDiff(ctx, dir, in.diff); reason != "" {
		return w.finishOptimize(ctx, id, StatusFailed, reason, names, nil, "")
	}
	if err := tasks.Untar(in.hiddenTar, hiddenDir); err != nil {
		return err
	}
	runner, ok := w.runner.(sandbox.OptimizeRunner)
	if !ok {
		return fmt.Errorf("submissions: the sandbox cannot run optimize tasks")
	}
	res, err := runner.RunOptimize(ctx, sandbox.OptimizeRequest{
		WorkDir: dir, CasesDir: filepath.Join(hiddenDir, "cases"), ScorerPath: filepath.Join(hiddenDir, "score.py"),
		Image: in.image, SolveCmd: in.solveCmd,
		CaseTimeout: time.Duration(in.caseTimeLimitS) * time.Second, Timeout: time.Duration(in.timeoutS) * time.Second})
	if err != nil {
		return err
	}
	if res.TimedOut {
		return w.finishOptimize(ctx, id, StatusFailed, ReasonTimeout, names, nil, "The run exceeded the total time limit.")
	}
	// Only cases of the catalog count, whatever the runner returned.
	byName := map[string]sandbox.CaseResult{}
	for _, c := range res.Cases {
		byName[c.Name] = c
	}
	cases := make([]sandbox.CaseResult, 0, len(names))
	valid := 0
	for _, n := range names {
		c, ok := byName[n]
		if !ok {
			c = sandbox.CaseResult{Name: n, Reason: "no result"}
		}
		if c.Valid {
			valid++
		}
		cases = append(cases, c)
	}
	status, reason := StatusPassed, ""
	if valid < len(names) {
		status, reason = StatusFailed, ReasonInvalidCases
	}
	return w.finishOptimize(ctx, id, status, reason, names, cases, "")
}

// finishOptimize writes the verdict. cases is nil when nothing was scored (score stays NULL).
func (w *Worker) finishOptimize(ctx context.Context, id, status, reason string, names []string, cases []sandbox.CaseResult, note string) error {
	tests := make([]TestResult, 0, len(names))
	var score *float64
	valid := 0
	logTail := note
	if cases != nil {
		sum := 0.0
		for _, c := range cases {
			t := TestResult{Name: c.Name, Passed: c.Valid, Reason: c.Reason}
			s := c.Score
			t.Score = &s
			tests = append(tests, t)
			sum += c.Score
			if c.Valid {
				valid++
			}
		}
		score = &sum
		logTail = fmt.Sprintf("Score %.4f. %d of %d cases valid.", sum, valid, len(cases))
		for _, c := range cases {
			if !c.Valid {
				logTail += fmt.Sprintf("\ncase %s: %s", c.Name, c.Reason)
			}
		}
	} else {
		for _, n := range names {
			tests = append(tests, TestResult{Name: n})
		}
	}
	logTail = sanitize.CleanLog(logTail, maxLogTail)
	testsJSON, err := json.Marshal(tests)
	if err != nil {
		return err
	}
	return w.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE submissions SET status = $2, failure_reason = NULLIF($3, ''), tests = $4, passed_tests = $5,
			cases_valid = $5, score = $6, log_tail = $7, finished_at = now() WHERE id = $1 AND status = 'running'`,
			id, status, reason, testsJSON, valid, score, logTail)
		return err
	})
}
