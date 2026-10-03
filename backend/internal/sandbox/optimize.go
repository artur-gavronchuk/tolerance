package sandbox

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ScorerImage runs the trusted score.py. It never sees the participant's code.
const ScorerImage = "arena-skill-python:1"

const maxCaseOutput = 1 << 20

// OptimizeRequest describes one run of an optimization task.
type OptimizeRequest struct {
	WorkDir     string // the repo with the participant's diff applied
	CasesDir    string // hidden NN.in files (host side, trusted)
	ScorerPath  string // hidden score.py (host side, trusted)
	Image       string // the task's language image
	SolveCmd    string
	CaseTimeout time.Duration
	Timeout     time.Duration // the whole solver run
}

type CaseResult struct {
	Name   string
	Score  float64
	Valid  bool
	Reason string
}

type OptimizeResult struct {
	Cases    []CaseResult
	TimedOut bool
}

// OptimizeRunner is the optimize half of Runner.
type OptimizeRunner interface {
	RunOptimize(ctx context.Context, req OptimizeRequest) (OptimizeResult, error)
}

func caseNames(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if e.Type().IsRegular() && strings.HasSuffix(e.Name(), ".in") {
			names = append(names, strings.TrimSuffix(e.Name(), ".in"))
		}
	}
	sort.Strings(names)
	return names, nil
}

// solveScript runs SOLVE_CMD per case. stdout goes to /out/NN.out (capped at 1 MB by ulimit -f), stderr is
// discarded, the exit status lands in /out/NN.status.
const solveScript = `mkdir -p /out; ulimit -f 2048
for f in /cases/*.in; do
  n=$(basename "$f" .in)
  timeout -s KILL "$CASE_T" sh -c "$SOLVE_CMD" < "$f" > "/out/$n.out" 2>/dev/null
  echo $? > "/out/$n.status"
done`

// scoreScript prints "NN<TAB>first line of score.py's stdout" per case.
const scoreScript = `for f in *.in; do
  n=${f%.in}
  printf '%s\t' "$n"
  timeout -s KILL 20 python3 score.py "$n.in" "$n.out" 2>/dev/null | head -n 1 | head -c 500
  echo
done`

func (d *Docker) container(ctx context.Context, image, workdir string, env []string, cmd ...string) (string, error) {
	raw, err := exec.CommandContext(ctx, "docker", createArgs(image, workdir, env, cmd...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("sandbox: docker create: %w: %s", err, strings.TrimSpace(string(raw)))
	}
	return strings.TrimSpace(string(raw)), nil
}

func cp(ctx context.Context, src, dst string) error {
	if out, err := exec.CommandContext(ctx, "docker", "cp", src, dst).CombinedOutput(); err != nil {
		return fmt.Errorf("sandbox: docker cp: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// start runs a created container to the end; timedOut reports a blown deadline.
func start(ctx context.Context, id string, timeout time.Duration) (out string, timedOut bool, err error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	w := &cappedWriter{limit: maxCapturedOutput}
	c := exec.CommandContext(runCtx, "docker", "start", "-a", id)
	c.Stdout, c.Stderr = w, w
	err = c.Run()
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return w.buf.String(), true, nil
	}
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		return "", false, fmt.Errorf("sandbox: docker start: %w", err)
	}
	if ee != nil && ee.ExitCode() == 125 {
		return "", false, fmt.Errorf("sandbox: container failed to start: %s", tail(w.buf.String(), 2000))
	}
	return w.buf.String(), false, nil
}

func (d *Docker) RunOptimize(ctx context.Context, req OptimizeRequest) (OptimizeResult, error) {
	names, err := caseNames(req.CasesDir)
	if err != nil {
		return OptimizeResult{}, err
	}
	// Container 1: the participant's code and the case inputs. No score.py.
	id, err := d.container(ctx, req.Image, "/work",
		[]string{"SOLVE_CMD=" + req.SolveCmd, "CASE_T=" + strconv.Itoa(int(req.CaseTimeout/time.Second))}, "sh", "-c", solveScript)
	if err != nil {
		return OptimizeResult{}, err
	}
	defer exec.Command("docker", "rm", "-f", id).Run() //nolint:errcheck
	if err := cp(ctx, req.WorkDir+"/.", id+":/work"); err != nil {
		return OptimizeResult{}, err
	}
	// /cases does not exist yet: copying a directory to a new path creates it.
	if err := cp(ctx, req.CasesDir, id+":/cases"); err != nil {
		return OptimizeResult{}, err
	}
	if _, timedOut, err := start(ctx, id, req.Timeout); err != nil {
		return OptimizeResult{}, err
	} else if timedOut {
		return OptimizeResult{TimedOut: true}, nil
	}
	outDir, err := os.MkdirTemp("", "arena-out-")
	if err != nil {
		return OptimizeResult{}, err
	}
	defer os.RemoveAll(outDir)
	if err := cp(ctx, id+":/out/.", outDir); err != nil {
		return OptimizeResult{}, err
	}

	// The copied files are untrusted: only regular files are read (no symlinks), capped in size.
	res := OptimizeResult{}
	var scorable []string
	idx := map[string]int{}
	stage, err := os.MkdirTemp("", "arena-score-")
	if err != nil {
		return OptimizeResult{}, err
	}
	defer os.RemoveAll(stage)
	for _, n := range names {
		c := CaseResult{Name: n}
		status, serr := readSmall(filepath.Join(outDir, n+".status"), 16)
		code, cerr := strconv.Atoi(strings.TrimSpace(string(status)))
		body, oerr := readSmall(filepath.Join(outDir, n+".out"), maxCaseOutput)
		switch {
		case serr != nil || cerr != nil:
			c.Reason = "the solver did not run"
		case code == 124 || code == 137 || code == 143:
			c.Reason = fmt.Sprintf("time limit exceeded (%ds)", int(req.CaseTimeout/time.Second))
		case code == 153:
			c.Reason = "output larger than 1 MB"
		case code != 0:
			c.Reason = fmt.Sprintf("the solver crashed (exit status %d)", code)
		case oerr != nil:
			c.Reason = "no usable output"
		case len(strings.TrimSpace(string(body))) == 0:
			c.Reason = "empty output"
		default:
			if err := os.WriteFile(filepath.Join(stage, n+".out"), body, 0o644); err != nil {
				return OptimizeResult{}, err
			}
			in, err := os.ReadFile(filepath.Join(req.CasesDir, n+".in"))
			if err != nil {
				return OptimizeResult{}, err
			}
			if err := os.WriteFile(filepath.Join(stage, n+".in"), in, 0o644); err != nil {
				return OptimizeResult{}, err
			}
			scorable = append(scorable, n)
		}
		idx[n] = len(res.Cases)
		res.Cases = append(res.Cases, c)
	}
	if len(scorable) == 0 {
		return res, nil
	}

	// Container 2: only score.py, the inputs from the catalog and the outputs. No participant code.
	scorer, err := os.ReadFile(req.ScorerPath)
	if err != nil {
		return OptimizeResult{}, err
	}
	if err := os.WriteFile(filepath.Join(stage, "score.py"), scorer, 0o644); err != nil {
		return OptimizeResult{}, err
	}
	sid, err := d.container(ctx, ScorerImage, "/score", nil, "sh", "-c", scoreScript)
	if err != nil {
		return OptimizeResult{}, err
	}
	defer exec.Command("docker", "rm", "-f", sid).Run() //nolint:errcheck
	if err := cp(ctx, stage+"/.", sid+":/score"); err != nil {
		return OptimizeResult{}, err
	}
	out, timedOut, err := start(ctx, sid, time.Duration(len(scorable)*25)*time.Second)
	if err != nil {
		return OptimizeResult{}, err
	}
	if timedOut {
		return OptimizeResult{}, fmt.Errorf("sandbox: scorer timed out")
	}
	lines := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if n, rest, ok := strings.Cut(l, "\t"); ok {
			lines[n] = strings.TrimSpace(rest)
		}
	}
	for _, n := range scorable {
		c := &res.Cases[idx[n]]
		c.Score, c.Valid, c.Reason = ParseScoreLine(lines[n])
	}
	return res, nil
}

func readSmall(path string, max int64) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > max {
		return nil, fmt.Errorf("not a regular file within %d bytes", max)
	}
	return os.ReadFile(path)
}

// ParseScoreLine reads one line of score.py output: a non-negative number, or "INVALID <reason>".
func ParseScoreLine(line string) (score float64, valid bool, reason string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return 0, false, "the scorer produced no result"
	}
	if rest, ok := strings.CutPrefix(line, "INVALID"); ok {
		rest = strings.TrimSpace(rest)
		if rest == "" {
			rest = "invalid output"
		}
		return 0, false, rest
	}
	v, err := strconv.ParseFloat(line, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0, false, "the scorer returned an unreadable result"
	}
	return v, true, ""
}

// RunOptimize on Fake returns the canned OptimizeResult.
func (f *Fake) RunOptimize(_ context.Context, req OptimizeRequest) (OptimizeResult, error) {
	f.OptimizeCalls = append(f.OptimizeCalls, req)
	return f.Optimize, f.Err
}

// RunOptimize on PassAll runs nothing: every case is valid with a deterministic score (1, 2, 3, ...).
func (PassAll) RunOptimize(_ context.Context, req OptimizeRequest) (OptimizeResult, error) {
	names, err := caseNames(req.CasesDir)
	if err != nil {
		return OptimizeResult{}, err
	}
	var res OptimizeResult
	for i, n := range names {
		res.Cases = append(res.Cases, CaseResult{Name: n, Score: float64(i + 1), Valid: true})
	}
	return res, nil
}
