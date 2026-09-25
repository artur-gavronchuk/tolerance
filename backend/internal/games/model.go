// Package games is the tanks tournament: bots and their versions, the ladder of matches between them, and
// the sync job that keeps the house bots and the tanks-bot proof task up to date. It imports proofs (an
// agent improves its bot the same way it improves a proof: by solving the tanks-bot task and submitting a
// diff), never the other way around.
package games

import (
	"log/slog"
	"os"
	"time"

	"tolerance/internal/games/match"
	"tolerance/internal/platform/db"
	"tolerance/internal/proofs"
)

// Game is the only game this platform runs today.
const Game = "tanks"

// Check is one automated check a bot version's package went through (entered by check_bot).
type Check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// VersionView is one bot_versions row as shown to its owner.
type VersionView struct {
	ID           string    `json:"id"`
	Number       int       `json:"number"`
	Source       string    `json:"source"`
	Status       string    `json:"status"`
	Language     string    `json:"language"`
	Checks       []Check   `json:"checks"`
	CheckLog     string    `json:"check_log"`
	CheckMatchID *string   `json:"check_match_id"`
	ProofID      *string   `json:"proof_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// MyBot is the caller's game_bots row, with the rating shown to people rather than the raw mu/sigma.
type MyBot struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Rating        int     `json:"rating"`
	Mu            float64 `json:"mu"`
	Sigma         float64 `json:"sigma"`
	Matches       int     `json:"matches"`
	Wins          int     `json:"wins"`
	ActiveVersion *int    `json:"active_version"`
}

// MyTanks is the whole /tanks/me page in one call.
type MyTanks struct {
	Bot       *MyBot         `json:"bot"`
	Versions  []VersionView  `json:"versions"`   // newest first, up to 20
	AgentRuns []proofs.Proof `json:"agent_runs"` // kind=game_bot proofs of the caller's agent, newest first, up to 10, without diff or log
	Matches   []MatchView    `json:"matches"`    // filled by Task 10; an empty, non-nil slice until then
}

// MatchPlayerView is one match_players row as shown alongside its match.
type MatchPlayerView struct {
	Slot         int    `json:"slot"`
	BotID        string `json:"bot_id"`
	Name         string `json:"name"`
	House        bool   `json:"house"`
	Source       string `json:"source"`
	Version      int    `json:"version"`
	Place        *int   `json:"place"`
	Kills        int    `json:"kills"`
	Damage       int    `json:"damage"`
	DeathTick    *int   `json:"death_tick"`
	Status       string `json:"status"`
	RatingBefore *int   `json:"rating_before"`
	RatingAfter  *int   `json:"rating_after"`
}

// MatchView is one matches row with its players, as shown on the ladder and on a bot's page.
type MatchView struct {
	ID         string            `json:"id"`
	Kind       string            `json:"kind"`
	Status     string            `json:"status"`
	Map        string            `json:"map"`
	Seed       int64             `json:"seed"`
	Ticks      int               `json:"ticks"`
	Featured   bool              `json:"featured"`
	HasReplay  bool              `json:"has_replay"`
	CreatedAt  time.Time         `json:"created_at"`
	StartedAt  *time.Time        `json:"started_at"`
	FinishedAt *time.Time        `json:"finished_at"`
	Players    []MatchPlayerView `json:"players"`
}

// Config tunes the games service. A zero Config gets sane defaults (see NewService).
type Config struct {
	CheckTicks int    // ticks a check_bot match runs for; 0 -> 600
	WorkDir    string // scratch directory bot archives are unpacked into to launch; "" -> os.TempDir()
}

func withDefaults(cfg Config) Config {
	if cfg.CheckTicks == 0 {
		cfg.CheckTicks = 600
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = os.TempDir()
	}
	return cfg
}

// Service is the tanks tournament: bots, versions, matches and the sync job.
type Service struct {
	pool   *db.Pool
	proofs *proofs.Service
	l      match.Launcher
	log    *slog.Logger
	cfg    Config
}

func NewService(pool *db.Pool, ps *proofs.Service, l match.Launcher, log *slog.Logger, cfg Config) *Service {
	return &Service{pool: pool, proofs: ps, l: l, log: log, cfg: withDefaults(cfg)}
}

// CheckBotPayload is the check_bot job's payload: the pending version to validate.
type CheckBotPayload struct {
	VersionID string `json:"version_id"`
}
