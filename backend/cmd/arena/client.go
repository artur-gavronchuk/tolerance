package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const version = "0.1.0"

type client struct {
	base string
	key  string
	http *http.Client
}

type taskInfo struct {
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	TaskMD        string `json:"task_md"`
	AgentTimeoutS int    `json:"agent_timeout_s"`
	RepoSHA256    string `json:"repo_sha256"`
}

type nextTask struct {
	ProofID string   `json:"proof_id"`
	Task    taskInfo `json:"task"`
}

type heartbeatResp struct {
	Agent struct {
		Name  string `json:"name"`
		Stage string `json:"stage"`
	} `json:"agent"`
}

type result struct {
	Diff       string `json:"diff"`
	LogTail    string `json:"log_tail"`
	DurationMS int    `json:"duration_ms"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"-"`
}

type apiError struct {
	Status int
	Code   string
	Msg    string
}

func (e *apiError) Error() string { return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Msg) }

func (c *client) do(ctx context.Context, method, path string, body any, out any) error {
	var buf bytes.Buffer
	if body != nil {
		enc := json.NewEncoder(&buf)
		// A diff can contain <, > or & (Go source, shell scripts, HTML...);
		// Go's default HTML-escaping of those in JSON strings inflates them
		// to < etc, which can push an otherwise in-limit diff over the
		// 1 MiB request body limit.
		enc.SetEscapeHTML(false)
		if err := enc.Encode(body); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "arena-connector/"+version)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		// The body was cut off (e.g. the connection dropped): as much a
		// network failure as no answer at all, and just as retryable.
		return err
	}
	if resp.StatusCode >= 400 {
		var p struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(raw, &p)
		return &apiError{Status: resp.StatusCode, Code: p.Code, Msg: p.Message}
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		if b, ok := out.(*[]byte); ok {
			*b = raw
		}
		return nil
	}
	if b, ok := out.(*[]byte); ok {
		*b = raw
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *client) Heartbeat(ctx context.Context) (heartbeatResp, error) {
	host, _ := os.Hostname()
	var out heartbeatResp
	err := c.do(ctx, http.MethodPost, "/api/v1/connector/heartbeat", map[string]string{"connector_version": version, "hostname": host}, &out)
	return out, err
}

// NextTask long-polls; nil, nil means nothing yet.
func (c *client) NextTask(ctx context.Context, wait time.Duration) (*nextTask, error) {
	var raw []byte
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/connector/tasks/next?wait=%d", int(wait.Seconds())), nil, &raw)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var t nextTask
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Repo downloads the task's repository, retrying like Result.
func (c *client) Repo(ctx context.Context, proofID string) ([]byte, error) {
	var raw []byte
	err := withRetry(ctx, func() error {
		return c.do(ctx, http.MethodGet, "/api/v1/connector/proofs/"+proofID+"/repo.tar.gz", nil, &raw)
	})
	return raw, err
}

func (c *client) Started(ctx context.Context, proofID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/connector/proofs/"+proofID+"/started", map[string]string{}, nil)
}

// Result sends the agent's result. Network failures and server errors are
// retried until ctx ends; a 409 means the server already has it; any other
// 4xx is final (413 diff_too_large has already ended the proof).
func (c *client) Result(ctx context.Context, proofID string, r result) error {
	return withRetry(ctx, func() error {
		err := c.do(ctx, http.MethodPost, "/api/v1/connector/proofs/"+proofID+"/result", r, nil)
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == http.StatusConflict {
			return nil
		}
		return err
	})
}

type statusResp struct {
	Agent struct {
		Name  string `json:"name"`
		Stage string `json:"stage"`
	} `json:"agent"`
	LastProof *struct {
		TaskSlug      string    `json:"task_slug"`
		Status        string    `json:"status"`
		FailureReason string    `json:"failure_reason"`
		CreatedAt     time.Time `json:"created_at"`
	} `json:"last_proof"`
}

// Status asks for the agent's stage and latest proof. It is not a
// heartbeat: running `arena status` never makes the agent look online.
func (c *client) Status(ctx context.Context) (statusResp, error) {
	var out statusResp
	err := c.do(ctx, http.MethodGet, "/api/v1/connector/status", nil, &out)
	return out, err
}

// retryBase is the first pause between attempts; tests shorten it.
var retryBase = 2 * time.Second

const retryCap = 30 * time.Second

// retryable reports whether another attempt could succeed: no usable answer
// came back, or the server had a temporary problem. Any other API error (a
// 4xx) is final.
func retryable(err error) bool {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae.Status >= 500 || ae.Status == http.StatusTooManyRequests
	}
	return true
}

// withRetry runs fn until it succeeds, fails for good, or ctx ends. The pause
// doubles from retryBase up to retryCap.
func withRetry(ctx context.Context, fn func() error) error {
	delay := retryBase
	for {
		err := fn()
		if err == nil || ctx.Err() != nil || !retryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(delay):
		}
		delay = min(delay*2, retryCap)
	}
}
