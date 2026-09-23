// Package checks runs the automated-check pipeline on the server side: it
// hands queued check runs to the checker process, validates and stores what
// comes back (results and evidence), and turns it into the submission's
// score. The browser work itself happens in the separate checker.
package checks

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"tolerance/fixtures/tasks"
	"tolerance/internal/platform/httpx"
)

// Run statuses.
const (
	RunQueued      = "queued"
	RunRunning     = "running"
	RunCompleted   = "completed"
	RunUnreachable = "unreachable"
	RunInfraError  = "infra_error"
)

const (
	maxEvidenceBytes    = 1 << 20  // one object
	maxRunEvidenceBytes = 8 << 20  // all objects of a run
	maxCompleteBody     = 12 << 20 // 8 MiB of evidence is about 10.7 MiB as base64
	maxTextField        = 1000
	maxDiagnostics      = 16 << 10
	maxLabel            = 200
	maxReason           = 500
)

var (
	evidenceKinds = map[string]bool{"screenshot": true, "page_text": true, "console": true, "network": true}
	contentTypes  = map[string]bool{"image/png": true, "image/jpeg": true, "text/plain": true, "application/json": true}
)

// ClaimResponse is what the checker receives for a run it should perform.
type ClaimResponse struct {
	JobID        string `json:"job_id"`
	CheckRunID   string `json:"check_run_id"`
	SubmissionID string `json:"submission_id"`
	PreviewURL   string `json:"preview_url"`
	Suite        string `json:"suite"`
	SuiteVersion string `json:"suite_version"`
	CommitSHA    string `json:"commit_sha"`
	RepoURL      string `json:"repo_url"`
}

// BuildInfo is what the deployed app declares at /arena-build.json.
type BuildInfo struct {
	Commit string `json:"commit"`
	Found  bool   `json:"found"`
}

type ResultInput struct {
	CheckID     string          `json:"check_id"`
	Status      string          `json:"status"`
	Expected    string          `json:"expected"`
	Actual      string          `json:"actual"`
	Diagnostics json.RawMessage `json:"diagnostics"`
	DurationMS  *int            `json:"duration_ms"`
}

type EvidenceInput struct {
	CheckID     string `json:"check_id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	ContentType string `json:"content_type"`
	Base64      string `json:"base64"`
}

// CompleteInput is the checker's report for one run.
type CompleteInput struct {
	Status            string          `json:"status"` // completed | unreachable | infra_error
	CheckerVersion    string          `json:"checker_version"`
	Browser           string          `json:"browser"`
	UnreachableReason string          `json:"unreachable_reason"`
	Error             string          `json:"error"`
	BuildInfo         *BuildInfo      `json:"build_info"`
	Results           []ResultInput   `json:"results"`
	Evidence          []EvidenceInput `json:"evidence"`
}

func unprocessable(msg, path, code string) error {
	return httpx.WithField(http.StatusUnprocessableEntity, "invalid_body", msg, path, code)
}

func truncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}

// decodedEvidence is evidence that passed validation.
type decodedEvidence struct {
	CheckID     string
	Kind        string
	Label       string
	ContentType string
	Bytes       []byte
}

// validate checks a report against the suite's manifest. It never trusts
// the checker for anything the manifest already says (titles, weights,
// which checks exist).
func (in CompleteInput) validate(b tasks.Bundle) ([]decodedEvidence, error) {
	switch in.Status {
	case RunCompleted, RunUnreachable, RunInfraError:
	default:
		return nil, unprocessable("status must be completed, unreachable or infra_error", "status", "invalid")
	}
	if in.Status == RunInfraError {
		return nil, nil // nothing else is read from a failed run
	}

	known := map[string]bool{}
	for _, c := range b.Checks {
		known[c.ID] = true
	}
	if in.Status == RunCompleted {
		if len(in.Results) != len(b.Checks) {
			return nil, unprocessable("results must contain exactly the checks of the suite, once each", "results", "wrong_set")
		}
		seen := map[string]bool{}
		for i, r := range in.Results {
			path := "results[" + strconv.Itoa(i) + "]"
			if !known[r.CheckID] || seen[r.CheckID] {
				return nil, unprocessable("unknown or repeated check id", path+".check_id", "wrong_set")
			}
			seen[r.CheckID] = true
			switch r.Status {
			case StatusPassed, StatusFailed, StatusInsufficientData, StatusInfraError:
			default:
				return nil, unprocessable("invalid result status", path+".status", "invalid")
			}
			if len(r.Diagnostics) > maxDiagnostics {
				return nil, unprocessable("diagnostics are too large", path+".diagnostics", "too_large")
			}
			if len(r.Diagnostics) > 0 {
				var obj map[string]json.RawMessage
				if json.Unmarshal(r.Diagnostics, &obj) != nil {
					return nil, unprocessable("diagnostics must be a JSON object", path+".diagnostics", "invalid")
				}
			}
			if r.DurationMS != nil && (*r.DurationMS < 0 || *r.DurationMS > 24*3600*1000) {
				return nil, unprocessable("duration_ms is out of range", path+".duration_ms", "out_of_range")
			}
		}
	}

	var out []decodedEvidence
	total := 0
	for i, e := range in.Evidence {
		path := "evidence[" + strconv.Itoa(i) + "]"
		if !evidenceKinds[e.Kind] || !contentTypes[e.ContentType] {
			return nil, unprocessable("unsupported evidence kind or content type", path, "unsupported")
		}
		if e.CheckID != "" && !known[e.CheckID] {
			return nil, unprocessable("evidence refers to an unknown check", path+".check_id", "wrong_set")
		}
		if e.Label == "" || len(e.Label) > maxLabel {
			return nil, unprocessable("evidence needs a label of at most 200 characters", path+".label", "invalid")
		}
		raw, err := base64.StdEncoding.DecodeString(e.Base64)
		if err != nil || len(raw) == 0 {
			return nil, unprocessable("evidence must be non-empty base64", path+".base64", "invalid")
		}
		if len(raw) > maxEvidenceBytes {
			return nil, unprocessable("evidence object is larger than 1 MiB", path+".base64", "too_large")
		}
		total += len(raw)
		if total > maxRunEvidenceBytes {
			return nil, unprocessable("evidence of a run may not exceed 8 MiB", "evidence", "too_large")
		}
		if !matchesContentType(e.ContentType, raw) {
			return nil, unprocessable("evidence bytes do not match the declared content type", path+".content_type", "mismatch")
		}
		out = append(out, decodedEvidence{CheckID: e.CheckID, Kind: e.Kind, Label: e.Label, ContentType: e.ContentType, Bytes: raw})
	}
	return out, nil
}

var (
	pngMagic  = []byte("\x89PNG\r\n\x1a\n")
	jpegMagic = []byte("\xff\xd8\xff")
)

// matchesContentType refuses bytes that do not look like what they claim to
// be: evidence is served back from our own origin.
func matchesContentType(contentType string, b []byte) bool {
	switch contentType {
	case "image/png":
		return bytes.HasPrefix(b, pngMagic)
	case "image/jpeg":
		return bytes.HasPrefix(b, jpegMagic)
	case "application/json":
		return json.Valid(b)
	default: // text/plain
		return utf8.Valid(b) && !bytes.Contains(b, []byte{0})
	}
}

// --- public views ---

type RunView struct {
	ID                string          `json:"id"`
	Status            string          `json:"status"`
	Suite             string          `json:"suite"`
	SuiteVersion      string          `json:"suite_version"`
	CheckerVersion    *string         `json:"checker_version"`
	Browser           *string         `json:"browser"`
	FunctionalScore   *int            `json:"functional_score"`
	UnreachableReason *string         `json:"unreachable_reason"`
	BuildInfo         json.RawMessage `json:"build_info"`
	CreatedAt         time.Time       `json:"created_at"`
	StartedAt         *time.Time      `json:"started_at"`
	FinishedAt        *time.Time      `json:"finished_at"`
}

type EvidenceRef struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	ContentType string `json:"content_type"`
	URL         string `json:"url"`
}

type OverrideView struct {
	Status   string `json:"status"`
	Reason   string `json:"reason"`
	ByHandle string `json:"by_handle"`
	At       time.Time `json:"at"`
}

type ResultView struct {
	CheckID         string          `json:"check_id"`
	Title           string          `json:"title"`
	Requirement     string          `json:"requirement"`
	Required        bool            `json:"required"`
	Weight          int             `json:"weight"`
	Status          string          `json:"status"`
	EffectiveStatus string          `json:"effective_status"`
	Expected        string          `json:"expected"`
	Actual          string          `json:"actual"`
	Diagnostics     json.RawMessage `json:"diagnostics"`
	DurationMS      *int            `json:"duration_ms"`
	Override        *OverrideView   `json:"override"`
	Evidence        []EvidenceRef   `json:"evidence"`
}

type ChecksView struct {
	Run         *RunView      `json:"run"`
	Results     []ResultView  `json:"results"`
	RunEvidence []EvidenceRef `json:"run_evidence"`
	History     []RunView     `json:"history"`
}
