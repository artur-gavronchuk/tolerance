// Package sandbox runs a prepared work directory inside a disposable
// container and reports which tests passed. A Runner error means the
// platform failed (no docker, no image), never that the code under test
// failed: that is reported inside Result.
package sandbox

import (
	"context"
	"time"
)

type Request struct {
	WorkDir string
	Image   string
	RunCmd  string
	Timeout time.Duration
}

type TestResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
}

type Result struct {
	Tests    []TestResult
	ExitCode int
	Output   string
	TimedOut bool
}

type Runner interface {
	Run(ctx context.Context, req Request) (Result, error)
}
