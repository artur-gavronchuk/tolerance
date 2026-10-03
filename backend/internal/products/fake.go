package products

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"tolerance/internal/sandbox"
)

// PassAll is the ARENA_SANDBOX=fake runner: it runs nothing and reports every scenario passed, so a local
// stack without Docker still reaches a score. It never executes the participant's code.
type PassAll struct{}

func (PassAll) Run(_ context.Context, req sandbox.Request) (sandbox.Result, error) {
	raw, err := os.ReadFile(filepath.Join(req.WorkDir, "_scenarios", "spec.json"))
	if err != nil {
		return sandbox.Result{}, err
	}
	var spec struct {
		Scenarios []Scenario `json:"scenarios"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return sandbox.Result{}, err
	}
	rs := make([]ScenarioResult, 0, len(spec.Scenarios))
	for _, sc := range spec.Scenarios {
		rs = append(rs, ScenarioResult{Name: sc.Name, Passed: true})
	}
	body, err := json.Marshal(rs)
	return sandbox.Result{Output: resultsMarker + string(body) + "\n"}, err
}

// RunOptimize is never used for product runs; it only completes sandbox.Runner.
func (PassAll) RunOptimize(ctx context.Context, req sandbox.OptimizeRequest) (sandbox.OptimizeResult, error) {
	return sandbox.PassAll{}.RunOptimize(ctx, req)
}
