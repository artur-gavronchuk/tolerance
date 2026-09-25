package games

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"tolerance/internal/agents"
	"tolerance/internal/games/botpkg"
	"tolerance/internal/games/rating"
	"tolerance/internal/platform/audit"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/idgen"
	"tolerance/internal/platform/jobs"
	"tolerance/internal/proofs"
)

const maxUploadsPerBotPerDay = 20

const botSelectCols = `g.id, g.name, g.mu, g.sigma, g.matches, g.wins, v.number`
const botSelectFrom = ` FROM game_bots g LEFT JOIN bot_versions v ON v.id = g.active_version_id`

func scanBot(row interface{ Scan(...any) error }) (MyBot, error) {
	var m MyBot
	var activeNumber *int
	if err := row.Scan(&m.ID, &m.Name, &m.Mu, &m.Sigma, &m.Matches, &m.Wins, &activeNumber); err != nil {
		return MyBot{}, err
	}
	m.ActiveVersion = activeNumber
	m.Rating = rating.Display(rating.Rating{Mu: m.Mu, Sigma: m.Sigma})
	return m, nil
}

const versionCols = `id, number, source, status, language, checks, check_log, check_match_id, proof_id, created_at`

func scanVersion(row interface{ Scan(...any) error }) (VersionView, error) {
	var v VersionView
	var checksRaw []byte
	if err := row.Scan(&v.ID, &v.Number, &v.Source, &v.Status, &v.Language, &checksRaw, &v.CheckLog, &v.CheckMatchID, &v.ProofID, &v.CreatedAt); err != nil {
		return VersionView{}, err
	}
	v.CreatedAt = v.CreatedAt.UTC()
	v.Checks = []Check{}
	if len(checksRaw) > 0 {
		if err := json.Unmarshal(checksRaw, &v.Checks); err != nil {
			return VersionView{}, err
		}
	}
	return v, nil
}

// isUniqueViolation reports whether err is a Postgres unique_violation, optionally naming which
// constraint (an empty want matches any unique violation).
func isUniqueViolation(err error, want string) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return false
	}
	return want == "" || pgErr.ConstraintName == want
}

// MyTanks assembles the caller's /tanks/me page: their bot (nil if they have none yet), its versions, its
// agent's tanks-bot proof history, and its recent matches (empty until Task 10 fills match history in).
func (s *Service) MyTanks(ctx context.Context, userID string) (MyTanks, error) {
	out := MyTanks{Versions: []VersionView{}, AgentRuns: []proofs.Proof{}, Matches: []MatchView{}}
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var botID string
		err := tx.QueryRow(ctx, `SELECT id FROM game_bots WHERE game = $1 AND owner_user_id = $2`, Game, userID).Scan(&botID)
		if errors.Is(err, pgx.ErrNoRows) {
			return s.loadAgentRuns(ctx, tx, userID, &out)
		}
		if err != nil {
			return err
		}
		bot, err := scanBot(tx.QueryRow(ctx, `SELECT `+botSelectCols+botSelectFrom+` WHERE g.id = $1`, botID))
		if err != nil {
			return err
		}
		out.Bot = &bot

		rows, err := tx.Query(ctx, `SELECT `+versionCols+` FROM bot_versions WHERE bot_id = $1 ORDER BY number DESC LIMIT 20`, botID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			v, err := scanVersion(rows)
			if err != nil {
				return err
			}
			out.Versions = append(out.Versions, v)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return s.loadAgentRuns(ctx, tx, userID, &out)
	})
	if err != nil {
		return MyTanks{}, err
	}
	return out, nil
}

// loadAgentRuns fills out.AgentRuns with the caller's agent's kind=game_bot proof history (newest first,
// up to 10, without diff or log). A caller with no agent yet is left with the empty slice out already has.
func (s *Service) loadAgentRuns(ctx context.Context, tx pgx.Tx, userID string, out *MyTanks) error {
	var agentID string
	err := tx.QueryRow(ctx, `SELECT id FROM agents WHERE owner_user_id = $1`, userID).Scan(&agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT id, agent_id, task_slug, status, created_at, claimed_at, diff_submitted_at, finished_at,
		agent_duration_ms, agent_exit_code, sandbox_result, failure_reason, kind
		FROM proofs WHERE agent_id = $1 AND kind = $2 ORDER BY created_at DESC LIMIT 10`, agentID, proofs.KindGameBot)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var p proofs.Proof
		if err := rows.Scan(&p.ID, &p.AgentID, &p.TaskSlug, &p.Status, &p.CreatedAt, &p.ClaimedAt, &p.DiffSubmittedAt, &p.FinishedAt,
			&p.AgentDurationMS, &p.AgentExitCode, &p.SandboxResult, &p.FailureReason, &p.Kind); err != nil {
			return err
		}
		p.CreatedAt = p.CreatedAt.UTC()
		if p.ClaimedAt != nil {
			u := p.ClaimedAt.UTC()
			p.ClaimedAt = &u
		}
		if p.DiffSubmittedAt != nil {
			u := p.DiffSubmittedAt.UTC()
			p.DiffSubmittedAt = &u
		}
		if p.FinishedAt != nil {
			u := p.FinishedAt.UTC()
			p.FinishedAt = &u
		}
		out.AgentRuns = append(out.AgentRuns, p)
	}
	return rows.Err()
}

// SaveBot creates the caller's bot or renames it. Name rules are the agent name regex
// (^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$). A name already taken by any bot (any case, house bots included) is a
// 409 name_taken rather than a 500 from the unique index; a name that doesn't match the pattern is a 422
// validation_failed with fields.name.
func (s *Service) SaveBot(ctx context.Context, userID, name string) (MyBot, error) {
	if !agents.NameRe.MatchString(name) {
		return MyBot{}, httpx.WithField(http.StatusUnprocessableEntity, "validation_failed",
			"name must match ^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$", "name", "invalid")
	}
	var m MyBot
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `SELECT id FROM game_bots WHERE game = $1 AND owner_user_id = $2`, Game, userID).Scan(&id)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			id = idgen.New("bot")
			if _, err := tx.Exec(ctx, `INSERT INTO game_bots (id, game, owner_user_id, name) VALUES ($1, $2, $3, $4)`, id, Game, userID, name); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "game_bot.created", AggregateKind: "game_bot", AggregateID: id, RequestID: httpx.RequestID(ctx)}); err != nil {
				return err
			}
		case err == nil:
			if _, err := tx.Exec(ctx, `UPDATE game_bots SET name = $2 WHERE id = $1`, id, name); err != nil {
				return err
			}
			if err := audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "game_bot.renamed", AggregateKind: "game_bot", AggregateID: id, RequestID: httpx.RequestID(ctx)}); err != nil {
				return err
			}
		default:
			return err
		}
		m, err = scanBot(tx.QueryRow(ctx, `SELECT `+botSelectCols+botSelectFrom+` WHERE g.id = $1`, id))
		return err
	})
	if isUniqueViolation(err, "") {
		return MyBot{}, httpx.New(http.StatusConflict, "name_taken", "That name is taken")
	}
	return m, err
}

// botForUpload returns the id of userID's bot, creating it from the archive's manifest name when the
// caller has none yet. A name collision on that auto-derived name (rather than one the owner chose
// explicitly through SaveBot) is reported as 409 no_bot, steering them to name their bot first.
func (s *Service) botForUpload(ctx context.Context, tx pgx.Tx, userID, manifestName string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM game_bots WHERE game = $1 AND owner_user_id = $2`, Game, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	id = idgen.New("bot")
	if _, err := tx.Exec(ctx, `INSERT INTO game_bots (id, game, owner_user_id, name) VALUES ($1, $2, $3, $4)`, id, Game, userID, manifestName); err != nil {
		if isUniqueViolation(err, "") {
			return "", httpx.New(http.StatusConflict, "no_bot", "Name your bot first")
		}
		return "", err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "game_bot.created", AggregateKind: "game_bot", AggregateID: id, RequestID: httpx.RequestID(ctx)}); err != nil {
		return "", err
	}
	return id, nil
}

// uploadVersion is the shared body of UploadVersion and UploadVersionForAgent: normalize the archive,
// resolve (or create) the owner's bot, enforce the daily upload limit, insert a pending version and
// enqueue its check.
func (s *Service) uploadVersion(ctx context.Context, userID string, archive []byte) (VersionView, error) {
	packed, m, err := botpkg.Normalize(archive)
	if err != nil {
		var pkgErr *botpkg.Error
		if errors.As(err, &pkgErr) {
			return VersionView{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_package", pkgErr.Msg, "archive", "invalid")
		}
		return VersionView{}, err
	}
	sum := sha256.Sum256(packed)
	sha := hex.EncodeToString(sum[:])

	var v VersionView
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		botID, err := s.botForUpload(ctx, tx, userID, m.Name)
		if err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM bot_versions WHERE bot_id = $1 AND source = 'upload' AND created_at > now() - interval '24 hours'`, botID).Scan(&count); err != nil {
			return err
		}
		if count >= maxUploadsPerBotPerDay {
			return httpx.New(http.StatusTooManyRequests, "upload_limit", "At most 20 uploads per bot per day")
		}
		var number int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(number), 0) + 1 FROM bot_versions WHERE bot_id = $1`, botID).Scan(&number); err != nil {
			return err
		}
		id := idgen.New("bv")
		v, err = scanVersion(tx.QueryRow(ctx, `INSERT INTO bot_versions (id, bot_id, number, source, language, entry, archive, archive_sha256, status)
			VALUES ($1, $2, $3, 'upload', $4, $5, $6, $7, 'pending') RETURNING `+versionCols,
			id, botID, number, m.Language, m.Entry, packed, sha))
		if err != nil {
			return err
		}
		if _, err := jobs.Enqueue(ctx, tx, "check_bot", CheckBotPayload{VersionID: id}, ""); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "bot_version.uploaded", AggregateKind: "bot_version", AggregateID: id, RequestID: httpx.RequestID(ctx)})
	})
	if err != nil {
		return VersionView{}, err
	}
	return v, nil
}

// UploadVersion normalizes archive (botpkg.Normalize), creates the caller's bot if they don't have one
// yet, enforces 20 uploads per bot per day, and inserts a pending version with the next number, enqueuing
// its check_bot job.
func (s *Service) UploadVersion(ctx context.Context, userID string, archive []byte) (VersionView, error) {
	return s.uploadVersion(ctx, userID, archive)
}

// UploadVersionForAgent is UploadVersion for the owner of agentID: the connector's `arena tanks submit`
// route authenticates as the agent, not as a session, so it resolves the owning user first.
func (s *Service) UploadVersionForAgent(ctx context.Context, agentID string, archive []byte) (VersionView, error) {
	var userID string
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT owner_user_id FROM agents WHERE id = $1`, agentID).Scan(&userID)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return VersionView{}, httpx.NotFound()
	}
	if err != nil {
		return VersionView{}, err
	}
	return s.uploadVersion(ctx, userID, archive)
}
