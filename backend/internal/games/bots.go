package games

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// maxBotNameLen matches agents.NameRe's own limit (a leading character plus up to 31 more).
const maxBotNameLen = 32

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
		matches, err := s.matchesTx(ctx, tx, botID, 20)
		if err != nil {
			return err
		}
		out.Matches = matches
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

// ensureBot returns the id of userID's bot, automatically creating and naming one when the caller has
// none yet - controller ruling: a manual "name your bot first" error is a poor experience here, since the
// starter kit's own bot.json defaults to "my-tank" and a second owner uploading it unmodified would always
// hit it. preferred (e.g. the archive's manifest name, then the caller's agent name) is tried in order;
// each candidate only counts if it already matches agents.NameRe. Owners can rename with SaveBot afterwards.
//
// It takes a transaction-scoped advisory lock keyed by userID before looking for an existing bot: two
// concurrent first-time uploads for the same user would otherwise both see "no bot yet", then race two
// INSERTs with two different (both individually free) names - the name uniqueness check below can't catch
// that, since neither name collides with the other; what collides is game_bots_owner_idx (UNIQUE (game,
// owner_user_id)), and the loser would abort the whole transaction with a raw unique-violation. The lock
// serializes bot creation per user, so the second caller's post-lock SELECT finds the first caller's bot
// and reuses it instead of racing to create a second one. pg_advisory_xact_lock is released automatically
// at the end of this transaction (commit or rollback), so there is nothing to unlock explicitly.
func (s *Service) ensureBot(ctx context.Context, tx pgx.Tx, userID string, preferred ...string) (string, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('games:bot:' || $1))`, userID); err != nil {
		return "", err
	}

	var id string
	err := tx.QueryRow(ctx, `SELECT id FROM game_bots WHERE game = $1 AND owner_user_id = $2`, Game, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	id, err = s.createBotWithName(ctx, tx, userID, preferred)
	if err != nil {
		return "", err
	}
	if err := audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: "game_bot.created", AggregateKind: "game_bot", AggregateID: id, RequestID: httpx.RequestID(ctx)}); err != nil {
		return "", err
	}
	return id, nil
}

// createBotWithName picks a free name for a new bot and inserts it: each of preferred that already matches
// agents.NameRe, tried plain and then suffixed "-2".."-99" (base truncated to keep the result within
// agents.NameRe's 32-character limit); if every one of those is taken, a random "tank-xxxxxx" name. Each
// attempt is race-safe against a concurrent caller picking a *different* name (INSERT ... ON CONFLICT
// DO NOTHING RETURNING id on the name index) - but not against a concurrent caller for the *same* user,
// which is what ensureBot's advisory lock is for; this function assumes that lock is already held.
func (s *Service) createBotWithName(ctx context.Context, tx pgx.Tx, userID string, preferred []string) (string, error) {
	var candidates []string
	for _, n := range preferred {
		if n != "" && agents.NameRe.MatchString(n) {
			candidates = append(candidates, n)
		}
	}
	for _, base := range candidates {
		if id, ok, err := tryCreateBot(ctx, tx, userID, base); err != nil {
			return "", err
		} else if ok {
			return id, nil
		}
		for n := 2; n <= 99; n++ {
			if id, ok, err := tryCreateBot(ctx, tx, userID, suffixedBotName(base, n)); err != nil {
				return "", err
			} else if ok {
				return id, nil
			}
		}
	}
	for attempt := 0; attempt < 20; attempt++ {
		name, err := randomBotName()
		if err != nil {
			return "", err
		}
		if id, ok, err := tryCreateBot(ctx, tx, userID, name); err != nil {
			return "", err
		} else if ok {
			return id, nil
		}
	}
	// Astronomically unlikely (20 random 6-character draws from a 36-character alphabet all colliding),
	// but report it as an ordinary internal error rather than a bare Go error leaking past the service
	// boundary.
	s.log.Error("games: could not find a free bot name", "user_id", userID)
	return "", httpx.Internal()
}

// suffixedBotName appends "-n" to base, truncating base first if needed so the result stays within
// agents.NameRe's 32-character limit (the suffix is always 2-3 characters, so the truncated base is always
// long enough to keep matching the pattern).
func suffixedBotName(base string, n int) string {
	suffix := fmt.Sprintf("-%d", n)
	if len(base)+len(suffix) > maxBotNameLen {
		base = base[:maxBotNameLen-len(suffix)]
	}
	return base + suffix
}

const botNameSuffixAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// randomBotName returns "tank-" plus 6 random lowercase alphanumeric characters, matching agents.NameRe.
func randomBotName() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, 6)
	for i, b := range buf {
		out[i] = botNameSuffixAlphabet[int(b)%len(botNameSuffixAlphabet)]
	}
	return "tank-" + string(out), nil
}

// tryCreateBot attempts to create one game_bots row with the given name, racing safely on the name index
// against any other transaction doing the same (ok is false, not an error, when the name is already
// taken). It does not by itself protect against two concurrent inserts for the same userID with two
// different, individually-free names - see ensureBot's advisory lock for that.
func tryCreateBot(ctx context.Context, tx pgx.Tx, userID, name string) (id string, ok bool, err error) {
	id = idgen.New("bot")
	err = tx.QueryRow(ctx, `INSERT INTO game_bots (id, game, owner_user_id, name) VALUES ($1, $2, $3, $4)
		ON CONFLICT (game, (lower(name))) DO NOTHING RETURNING id`, id, Game, userID, name).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// agentNameFor returns the name of userID's agent, or "" if they don't have one yet.
func (s *Service) agentNameFor(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var name string
	err := tx.QueryRow(ctx, `SELECT name FROM agents WHERE owner_user_id = $1`, userID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// createVersion is the shared body of uploadVersion (source 'upload') and JudgeProof (source 'agent'):
// resolve (or create) the owner's bot, enforce the daily upload limit (uploads only - an agent run is
// already limited by proofs' own daily cap), insert a pending version with the given source and optional
// proofID, and enqueue its check_bot job for an upload. An agent run's check runs synchronously right
// after this call (see JudgeProof), so it does not enqueue a second, redundant check_bot job for the same
// version - Qualify is idempotent either way, but there is no reason to spend a second check match on it.
//
// When proofID is set, this call is retry-safe: a game_bot proof's run_proof job can retry after a
// platform failure (docker down mid check match, ...), and each retry re-extracts and re-applies the same
// diff and calls JudgeProof again. Before inserting, it looks for a version already carrying this
// proofID for this bot and, if one exists, returns that row untouched instead of inserting a second one -
// whether that row is still 'pending' (the caller's next Qualify call will actually run the check, or run
// it again) or already resolved ('active'/'rejected', in which case Qualify just hands back the stored
// result - see its own doc comment on why that's idempotent). This is race-safe against two concurrent
// retries for the same proof because it runs after ensureBot's per-user advisory lock (held for the rest
// of this transaction whether or not it had to create a bot) and the FOR UPDATE below, both scoped to this
// same user/bot: a second transaction's lookup can't run until the first one's insert (or no-op) has
// committed.
func (s *Service) createVersion(ctx context.Context, userID string, packed []byte, m botpkg.Manifest, source string, proofID *string) (VersionView, error) {
	sum := sha256.Sum256(packed)
	sha := hex.EncodeToString(sum[:])

	var v VersionView
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentName, err := s.agentNameFor(ctx, tx, userID)
		if err != nil {
			return err
		}
		botID, err := s.ensureBot(ctx, tx, userID, m.Name, agentName)
		if err != nil {
			return err
		}
		// Lock the bot row for the rest of this transaction before computing the next version number, so
		// two concurrent version creations for the same bot serialize instead of both computing the same
		// coalesce(max(number), 0) + 1 and racing on the (bot_id, number) unique constraint.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM game_bots WHERE id = $1 FOR UPDATE`, botID); err != nil {
			return err
		}
		if proofID != nil {
			existing, err := scanVersion(tx.QueryRow(ctx, `SELECT `+versionCols+` FROM bot_versions WHERE bot_id = $1 AND proof_id = $2`, botID, *proofID))
			if err == nil {
				v = existing
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if source == "upload" {
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM bot_versions WHERE bot_id = $1 AND source = 'upload' AND created_at > now() - interval '24 hours'`, botID).Scan(&count); err != nil {
				return err
			}
			if count >= maxUploadsPerBotPerDay {
				return httpx.New(http.StatusTooManyRequests, "upload_limit", "At most 20 uploads per bot per day")
			}
		}
		var number int
		if err := tx.QueryRow(ctx, `SELECT coalesce(max(number), 0) + 1 FROM bot_versions WHERE bot_id = $1`, botID).Scan(&number); err != nil {
			return err
		}
		id := idgen.New("bv")
		v, err = scanVersion(tx.QueryRow(ctx, `INSERT INTO bot_versions (id, bot_id, number, source, language, entry, archive, archive_sha256, proof_id, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending') RETURNING `+versionCols,
			id, botID, number, source, m.Language, m.Entry, packed, sha, proofID))
		if err != nil {
			return err
		}
		action := "bot_version.uploaded"
		if source == "upload" {
			if _, err := jobs.Enqueue(ctx, tx, "check_bot", CheckBotPayload{VersionID: id}, ""); err != nil {
				return err
			}
		} else {
			action = "bot_version.agent_run"
		}
		return audit.Record(ctx, tx, audit.Event{ActorID: userID, Action: action, AggregateKind: "bot_version", AggregateID: id, RequestID: httpx.RequestID(ctx)})
	})
	if isUniqueViolation(err, "bot_versions_bot_id_number_key") || isUniqueViolation(err, "game_bots_owner_idx") {
		// Backstop: the FOR UPDATE lock (version numbering) and the advisory lock in ensureBot (first bot
		// creation) should make both of these unreachable, but a raw 500 from the database is still worse
		// than telling the caller to just retry the upload.
		return VersionView{}, httpx.New(http.StatusConflict, "upload_conflict", "Another upload is in progress; try again")
	}
	if err != nil {
		return VersionView{}, err
	}
	return v, nil
}

// uploadVersion is the shared body of UploadVersion and UploadVersionForAgent: normalize the archive
// (botpkg.Normalize) and hand it to createVersion as source 'upload'.
func (s *Service) uploadVersion(ctx context.Context, userID string, archive []byte) (VersionView, error) {
	packed, m, err := botpkg.Normalize(archive)
	if err != nil {
		var pkgErr *botpkg.Error
		if errors.As(err, &pkgErr) {
			return VersionView{}, httpx.WithField(http.StatusUnprocessableEntity, "invalid_package", pkgErr.Msg, "archive", "invalid")
		}
		return VersionView{}, err
	}
	return s.createVersion(ctx, userID, packed, m, "upload", nil)
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
