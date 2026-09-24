package agents

import (
	"regexp"
	"time"
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$`)

type Agent struct {
	ID          string
	OwnerUserID string
	Name        string
	Description string
	CreatedAt   time.Time
	Version     int
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
}

type CreateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type PatchInput struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type Presence struct {
	LastSeenAt       time.Time `json:"last_seen_at"`
	ConnectorVersion string    `json:"connector_version"`
	Hostname         string    `json:"hostname"`
}

type Overview struct {
	Private
	Stage    string    `json:"stage"`
	Presence *Presence `json:"presence"`
}

type heartbeatInput struct {
	ConnectorVersion string `json:"connector_version"`
	Hostname         string `json:"hostname"`
}
