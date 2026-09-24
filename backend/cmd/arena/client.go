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
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
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
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
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

func (c *client) Repo(ctx context.Context, proofID string) ([]byte, error) {
	var raw []byte
	err := c.do(ctx, http.MethodGet, "/api/v1/connector/proofs/"+proofID+"/repo.tar.gz", nil, &raw)
	return raw, err
}

func (c *client) Started(ctx context.Context, proofID string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/connector/proofs/"+proofID+"/started", map[string]string{}, nil)
}

// Result retries on network errors; a 409 means the server already has it.
func (c *client) Result(ctx context.Context, proofID string, r result) error {
	delay := 2 * time.Second
	for attempt := 1; ; attempt++ {
		err := c.do(ctx, http.MethodPost, "/api/v1/connector/proofs/"+proofID+"/result", r, nil)
		var ae *apiError
		if err == nil || (errors.As(err, &ae) && ae.Status == http.StatusConflict) {
			return nil
		}
		if errors.As(err, &ae) || attempt >= 6 {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}
