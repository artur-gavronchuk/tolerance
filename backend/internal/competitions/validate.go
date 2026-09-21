package competitions

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"tolerance/fixtures/tasks"
	"tolerance/internal/platform/httpx"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

var Categories = map[string]bool{"Full build": true, "Bug fix": true, "DB design": true, "Refactor": true, "Integration": true}
var Difficulties = map[string]bool{"Easy": true, "Medium": true, "Hard": true}

func field(msg, path, code string) error {
	return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", msg, path, code)
}

// Validate enforces invariant 3 of the design (criteria) plus field limits.
func Validate(in Input, now time.Time) error {
	switch {
	case !slugRe.MatchString(in.Slug):
		return field("slug must match ^[a-z0-9][a-z0-9-]{1,63}$", "slug", "invalid")
	case !httpx.ValidText(in.Title, 1, 200):
		return field("title must be 1-200 characters", "title", "invalid")
	case !httpx.ValidText(in.Summary, 1, 500):
		return field("summary must be 1-500 characters", "summary", "invalid")
	case !httpx.ValidText(in.Brief, 1, 10000):
		return field("brief must be 1-10000 characters", "brief", "invalid")
	case !Categories[in.Category]:
		return field("category must be one of Full build, Bug fix, DB design, Refactor, Integration", "category", "invalid")
	case !Difficulties[in.Difficulty]:
		return field("difficulty must be Easy, Medium or Hard", "difficulty", "invalid")
	case in.Points < 1 || in.Points > 10000:
		return field("points must be 1-10000", "points", "invalid")
	case !in.Deadline.After(now):
		return field("deadline must be in the future", "deadline", "past")
	case in.MatchDurationSeconds < 300 || in.MatchDurationSeconds > 3600:
		return field("match_duration_seconds must be 300-3600", "match_duration_seconds", "invalid")
	case len(in.Criteria) < 1 || len(in.Criteria) > 10:
		return field("criteria must have 1-10 items", "criteria", "invalid")
	}
	sum := 0
	seen := map[string]bool{}
	checksCriteria := 0
	for i, c := range in.Criteria {
		name := strings.TrimSpace(c.Name)
		if !httpx.ValidText(name, 1, 40) || !httpx.ValidText(c.Description, 1, 300) {
			return field("criterion name (1-40) and description (1-300) are required", "criteria", "invalid")
		}
		if seen[name] {
			return field("criterion names must be unique", "criteria", "duplicate")
		}
		seen[name] = true
		if c.Weight < 1 {
			return field("criterion weight must be at least 1", "criteria", "invalid")
		}
		switch c.Source {
		case "":
			in.Criteria[i].Source = SourceLLM
		case SourceLLM:
		case SourceChecks:
			checksCriteria++
		default:
			return field("criterion source must be checks or llm", "criteria", "invalid_source")
		}
		sum += c.Weight
		in.Criteria[i].Name = name
	}
	if sum != 100 {
		return field("criterion weights must sum to 100", "criteria", "weights_sum")
	}
	return validateSuite(in, checksCriteria)
}

// validateSuite ties the check suite, the published task and the criterion
// measured by checks together: either all three are present or none.
func validateSuite(in Input, checksCriteria int) error {
	if in.CheckSuite == "" {
		if checksCriteria > 0 {
			return field("a criterion with source checks requires a check_suite", "criteria", "checks_without_suite")
		}
		if len(in.Task) > 0 {
			return field("task requires a check_suite", "task", "task_without_suite")
		}
		return nil
	}
	if _, err := tasks.Load(in.CheckSuite); err != nil {
		return field("check_suite is not a known task bundle", "check_suite", "unknown")
	}
	var task map[string]json.RawMessage
	if len(in.Task) == 0 || json.Unmarshal(in.Task, &task) != nil || len(task) == 0 {
		return field("task must be the published task object for the check_suite", "task", "invalid")
	}
	if checksCriteria != 1 {
		return field("exactly one criterion must have source checks when a check_suite is set", "criteria", "checks_criterion")
	}
	return nil
}
