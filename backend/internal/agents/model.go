package agents

import (
	"regexp"
	"time"

	"tolerance/internal/skillrating"
)

// NameRe is the shared agent/bot name pattern: alphanumeric, hyphen and underscore, 2-32 characters,
// starting with a letter or digit. The games package reuses it for game_bots.name.
var NameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$`)

type Agent struct {
	ID          string
	OwnerUserID string
	Name        string
	Description string
	CreatedAt   time.Time
	Version     int
	Public      bool
	BannedAt    *time.Time
}

type KeyView struct {
	ID         string     `json:"id"`
	Prefix     string     `json:"prefix"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type Private struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	APIKeys     []KeyView `json:"api_keys"`
	// Public is whether this agent appears in the arena's public tables and has
	// a public profile. It does not affect whether ratings are computed.
	Public bool `json:"public"`
}

type CreateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PatchInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	// Public is a pointer so a patch that omits it leaves the setting alone: a
	// rename must not silently republish an agent the owner hid.
	Public *bool `json:"public"`
}

type Presence struct {
	LastSeenAt       time.Time `json:"last_seen_at"`
	ConnectorVersion string    `json:"connector_version"`
	Hostname         string    `json:"hostname"`
}

type Overview struct {
	Private
	Stage    string                    `json:"stage"`
	Presence *Presence                 `json:"presence"`
	Version  *Version                  `json:"version"`
	Skills   []skillrating.SkillRating `json:"skills"`
}

type heartbeatInput struct {
	ConnectorVersion string        `json:"connector_version"`
	Hostname         string        `json:"hostname"`
	Version          *VersionInput `json:"version"`
}
