package missions

import (
	"encoding/json"
	"time"
)

type Requirement struct {
	ID               string `json:"id"`
	MissionVersionID string `json:"mission_version_id"`
	StableKey        string `json:"stable_key"`
	Revision         int    `json:"revision"`
	Gate             bool   `json:"gate"`
	Weight           int    `json:"weight"`
	Category         string `json:"category"`
	Text             string `json:"text"`
}

type MissionVersion struct {
	ID                     string          `json:"id"`
	OrganizationID         string          `json:"organization_id"`
	MissionID              string          `json:"mission_id"`
	Number                 int             `json:"number"`
	ParentVersionID        *string         `json:"parent_version_id,omitempty"`
	Contract               json.RawMessage `json:"contract"`
	ContractDigest         *string         `json:"contract_digest,omitempty"`
	CalibrationConfirmedAt *time.Time      `json:"calibration_confirmed_at,omitempty"`
	CalibrationConfirmedBy *string         `json:"calibration_confirmed_by,omitempty"`
	PublishedAt            *time.Time      `json:"published_at,omitempty"`
	SubmissionDeadline     *time.Time      `json:"submission_deadline,omitempty"`
	AppealDeadline         *time.Time      `json:"appeal_deadline,omitempty"`
	Policies               json.RawMessage `json:"policies"`
	Version                int             `json:"version"`
	Requirements           []Requirement   `json:"requirements"`
}

type Mission struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organization_id"`
	CampaignID      string          `json:"campaign_id"`
	Stage           string          `json:"stage"`
	Ordinal         int             `json:"ordinal"`
	State           string          `json:"state"`
	ActiveVersionID *string         `json:"active_version_id,omitempty"`
	Version         int             `json:"version"`
	CurrentVersion  *MissionVersion `json:"current_version,omitempty"`
}

const (
	StateDraft            = "draft"
	StateCalibrating      = "calibrating"
	StateOpen             = "open"
	StateSubmissionClosed = "submission_closed"
	StateEvaluating       = "evaluating"
	StateProvisional      = "provisional"
	StateAppealsOpen      = "appeals_open"
	StateFinalized        = "finalized"
	StateCancelled        = "cancelled"
	StateInvalidated      = "invalidated"
)

// evaluatorVersion identifies the scenario logic that will grade
// submissions against this contract. Slice 2 wires up the actual evaluator
// (internal/routecheck for the pilot season); the value is recorded on the
// contract now so a mission opened in slice 1 already names what will
// check it.
const evaluatorVersion = "route-v1"

type contract struct {
	Title    string    `json:"title"`
	Brief    string    `json:"brief"`
	Deadline time.Time `json:"deadline"`
}

// requirementDigest carries only the content that makes a requirement a
// commitment, not its storage identity: two requirements with the same
// stable_key, revision and text are the same promise even if their
// database ids (generated randomly) differ, including across a from-scratch
// re-import of the same contract.
type requirementDigest struct {
	StableKey string `json:"stable_key"`
	Revision  int    `json:"revision"`
	Gate      bool   `json:"gate"`
	Weight    int    `json:"weight"`
	Category  string `json:"category"`
	Text      string `json:"text"`
}

// digestPayload is exactly the set of fields that make two contracts the
// same commitment. Operational fields (mission state, calibration
// timestamps, version's own row version counter, requirement/version row
// ids) never contribute: they describe where the process is or how it is
// stored, not what was promised.
type digestPayload struct {
	MissionID        string              `json:"mission_id"`
	Number           int                 `json:"number"`
	Contract         contract            `json:"contract"`
	Requirements     []requirementDigest `json:"requirements"`
	EvaluatorVersion string              `json:"evaluator_version"`
}
