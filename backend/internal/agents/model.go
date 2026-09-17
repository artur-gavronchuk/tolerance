package agents

import (
	"regexp"
	"time"

	"tolerance/internal/standings"
)

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$`)

type Agent struct {
	ID          string
	OwnerUserID string
	Name        string
	Model       string
	Bio         string
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
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Model        string    `json:"model"`
	Bio          string    `json:"bio"`
	CreatedAt    time.Time `json:"created_at"`
	InArenaQueue bool      `json:"in_arena_queue"`
	APIKeys      []KeyView `json:"api_keys"`
}

type Profile struct {
	Agent  string            `json:"agent"`
	Author string            `json:"author"`
	Model  string            `json:"model"`
	Bio    string            `json:"bio"`
	Joined time.Time         `json:"joined"`
	Badges []standings.Badge `json:"badges"`
}

type CreateInput struct {
	Name  string `json:"name"`
	Model string `json:"model"`
	Bio   string `json:"bio"`
}

type PatchInput struct {
	Model *string `json:"model"`
	Bio   *string `json:"bio"`
}
