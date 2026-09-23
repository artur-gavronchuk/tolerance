// Package attempts records one run of an agent at a competition: official
// (one per competition and agent) or practice (any number). Each attempt
// keeps the agent configuration it started with and a log of safe events.
package attempts

import (
	"encoding/json"
	"time"
)

const (
	KindOfficial = "official"
	KindPractice = "practice"

	StatusRunning   = "running"
	StatusSubmitted = "submitted"
	StatusAbandoned = "abandoned"
)

// Event kinds a connector may send; the rest are written by the server.
var connectorKinds = map[string]bool{
	"started": true, "phase": true, "log": true, "preview_available": true, "build_finished": true,
}

var serverKinds = map[string]bool{
	"submitted": true, "check_started": true, "check_finished": true, "result": true, "abandoned": true,
}

const (
	maxFieldLen       = 80
	maxEventsPerBatch = 20
	maxPhaseIndex     = 8
)

// AgentConfig is what the connector reports about how the agent is run.
type AgentConfig struct {
	Adapter          string `json:"adapter"`
	AdapterModel     string `json:"adapter_model"`
	ConnectorVersion string `json:"connector_version"`
	OS               string `json:"os"`
}

// Snapshot is the configuration frozen at attempt start: the profile as it
// was at that moment plus the connector's report.
type Snapshot struct {
	Name             string `json:"name"`
	Model            string `json:"model"`
	Bio              string `json:"bio"`
	Adapter          string `json:"adapter"`
	AdapterModel     string `json:"adapter_model"`
	ConnectorVersion string `json:"connector_version"`
	OS               string `json:"os"`
}

type Attempt struct {
	ID              string          `json:"id"`
	CompetitionID   string          `json:"-"`
	CompetitionSlug string          `json:"competition_slug"`
	AgentID         string          `json:"-"`
	Kind            string          `json:"kind"`
	No              int             `json:"no"`
	Status          string          `json:"status"`
	AgentSnapshot   json.RawMessage `json:"agent_snapshot"`
	MatchID         *string         `json:"match_id"`
	StartedAt       time.Time       `json:"started_at"`
	FinishedAt      *time.Time      `json:"finished_at"`
	Voided          bool            `json:"voided"`
}

type Event struct {
	ID         int64           `json:"id"`
	Kind       string          `json:"kind"`
	PhaseIndex *int            `json:"phase_index"`
	Text       *string         `json:"text"`
	Payload    json.RawMessage `json:"payload"`
	At         time.Time       `json:"at"`
}

// EventInput is one event as sent by a connector.
type EventInput struct {
	Kind       string `json:"kind"`
	PhaseIndex *int   `json:"phase_index"`
	Text       string `json:"text"`
}
