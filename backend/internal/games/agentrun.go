package games

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/botpkg"
	"tolerance/internal/games/tanks"
	"tolerance/internal/platform/sanitize"
	"tolerance/internal/proofs"
)

// tanksBotCheckLogLines and tanksBotCheckLogCapBytes bound how much of a check_log RESULTS.md and
// JudgeProof's Output embed: it is the bot's own stderr, sanitized but still arbitrary text the bot
// printed, so both a line count and a byte cap keep either document from growing unbounded.
const tanksBotCheckLogLines = 30
const tanksBotCheckLogCapBytes = 4 << 10

// StartAgentRun creates the caller's bot if missing (named after the agent), builds the task repository -
// files of the active version (or the Python starter), GAME.md and RESULTS.md - and creates a game_bot
// proof with it. Errors from proofs (agent_offline, proof_in_progress, daily_limit, no_agent) pass
// through unchanged: this is a thin wrapper around proofs.CreateWithRepo, not a second set of the same
// checks.
//
// ensureBot and the read of the bot's active version run in the same transaction (a controller ruling for
// this task): a bot created by this call must exist by the time its active version is looked up, and there
// is nothing else in between that could observe it half-done.
func (s *Service) StartAgentRun(ctx context.Context, userID string) (proofs.Proof, error) {
	var repoTar []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		agentName, err := s.agentNameFor(ctx, tx, userID)
		if err != nil {
			return err
		}
		botID, err := s.ensureBot(ctx, tx, userID, agentName)
		if err != nil {
			return err
		}
		files, err := s.activeVersionFiles(ctx, tx, botID)
		if err != nil {
			return err
		}
		results, err := s.resultsMD(ctx, tx, botID)
		if err != nil {
			return err
		}
		files["GAME.md"] = []byte(tanks.GameMD)
		files["RESULTS.md"] = []byte(results)
		repoTar, err = proofs.TarFiles(files)
		return err
	})
	if err != nil {
		return proofs.Proof{}, err
	}
	return s.proofs.CreateWithRepo(ctx, userID, tanksBotSlug, repoTar)
}

// activeVersionFiles returns botID's active version unpacked into a path -> contents map, or the Python
// starter kit for a bot that has never qualified a version yet.
func (s *Service) activeVersionFiles(ctx context.Context, tx pgx.Tx, botID string) (map[string][]byte, error) {
	var archive []byte
	err := tx.QueryRow(ctx, `SELECT bv.archive FROM game_bots gb JOIN bot_versions bv ON bv.id = gb.active_version_id
		WHERE gb.id = $1`, botID).Scan(&archive)
	if errors.Is(err, pgx.ErrNoRows) {
		return tanks.Starter("python")
	}
	if err != nil {
		return nil, err
	}
	return unpackArchiveToFiles(archive)
}

// unpackArchiveToFiles reads a bot archive (as produced by botpkg.Normalize/PackDir: deterministic,
// regular files only, no directory entries) into a path -> contents map. It trusts the archive's shape
// because it only ever reads what this service itself already validated and stored.
func unpackArchiveToFiles(archive []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("games: unpack bot archive: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("games: unpack bot archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("games: unpack bot archive: %w", err)
		}
		files[h.Name] = data
	}
	return files, nil
}

// JudgeProof implements proofs.GameBotJudge: dir is the tanks-bot proof's repo with the agent's diff
// already applied. Pack it (PackDir already strips GAME.md/RESULTS.md/TASK.md/arena-agent.log, so those
// never end up back in the bot package); a *botpkg.Error means the package itself is broken and the
// verdict is failed/invalid_package without ever touching the database. Otherwise create a pending version
// (source 'agent', proof_id = proofID) and qualify it synchronously, the same check an upload gets.
//
// The verdict's Passed comes from the checks themselves, not from Qualify's own bool return or the
// version's stored status: a version that passed every check can still end up status = 'rejected' if a
// newer version of the same bot was activated first (Qualify's "supersede" case - see its own doc
// comment). That is bookkeeping about which version is current, not a verdict on this diff, so it must not
// turn a working bot into a reported failure; checksTrulyPassed treats a "supersede" entry as the
// bookkeeping it is, ignoring it for pass/fail, while still leaving it in Tests so the record of it isn't
// lost.
func (s *Service) JudgeProof(ctx context.Context, proofID, agentID, dir string) (proofs.GameBotVerdict, error) {
	archive, m, err := botpkg.PackDir(dir)
	if err != nil {
		var pkgErr *botpkg.Error
		if errors.As(err, &pkgErr) {
			return proofs.GameBotVerdict{
				Passed: false,
				Reason: "invalid_package",
				Tests:  []proofs.TestResult{{Name: "package", Passed: false}},
				Output: pkgErr.Msg,
			}, nil
		}
		return proofs.GameBotVerdict{}, err
	}

	var userID string
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT owner_user_id FROM agents WHERE id = $1`, agentID).Scan(&userID)
	})
	if err != nil {
		return proofs.GameBotVerdict{}, err
	}

	v, err := s.createVersion(ctx, userID, archive, m, "agent", &proofID)
	if err != nil {
		return proofs.GameBotVerdict{}, err
	}

	_, checks, err := s.Qualify(ctx, v.ID)
	if err != nil {
		return proofs.GameBotVerdict{}, err
	}

	var checkLog string
	err = s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT check_log FROM bot_versions WHERE id = $1`, v.ID).Scan(&checkLog)
	})
	if err != nil {
		return proofs.GameBotVerdict{}, err
	}

	passed := checksTrulyPassed(checks)
	reason := ""
	if !passed {
		reason = "bot_rejected"
	}
	tests := make([]proofs.TestResult, len(checks))
	var out strings.Builder
	for i, c := range checks {
		tests[i] = proofs.TestResult{Name: c.Name, Passed: c.Passed}
		fmt.Fprintf(&out, "%s: %s\n", c.Name, c.Detail)
	}
	if checkLog != "" {
		out.WriteString("\n")
		out.WriteString(sanitize.CleanLog(checkLog, tanksBotCheckLogCapBytes))
	}

	return proofs.GameBotVerdict{Passed: passed, Reason: reason, Tests: tests, Output: out.String()}, nil
}

// checksTrulyPassed is checksPassed with one exception: a "supersede" entry (Qualify's own bookkeeping for
// a version that passed its check but was beaten to activation by a newer one - see Qualify's doc comment)
// never counts against the verdict. Everything else must still have passed.
func checksTrulyPassed(checks []Check) bool {
	for _, c := range checks {
		if c.Name == "supersede" {
			continue
		}
		if !c.Passed {
			return false
		}
	}
	return true
}

// resultsMD renders RESULTS.md: the bot's name, rating and match record, the active version's checks (if
// any) with the first tanksBotCheckLogLines lines of its check log, and the bot's last 20 finished ladder
// matches (place, kills, damage, status, opponents). English markdown; a bot with no history at all gets a
// short "no matches yet" file rather than a mostly-empty document.
func (s *Service) resultsMD(ctx context.Context, tx pgx.Tx, botID string) (string, error) {
	bot, err := scanBot(tx.QueryRow(ctx, `SELECT `+botSelectCols+botSelectFrom+` WHERE g.id = $1`, botID))
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Results for %s\n\n", bot.Name)
	fmt.Fprintf(&sb, "Rating: %d (%d matches, %d wins)\n\n", bot.Rating, bot.Matches, bot.Wins)

	if bot.ActiveVersion == nil {
		sb.WriteString("No version has qualified yet.\n\n")
	} else {
		var checksRaw []byte
		var checkLog string
		err := tx.QueryRow(ctx, `SELECT checks, check_log FROM bot_versions WHERE bot_id = $1 AND number = $2`,
			botID, *bot.ActiveVersion).Scan(&checksRaw, &checkLog)
		if err != nil {
			return "", err
		}
		var checks []Check
		if len(checksRaw) > 0 {
			if err := json.Unmarshal(checksRaw, &checks); err != nil {
				return "", err
			}
		}
		fmt.Fprintf(&sb, "## Active version %d checks\n\n", *bot.ActiveVersion)
		for _, c := range checks {
			status := "FAIL"
			if c.Passed {
				status = "PASS"
			}
			fmt.Fprintf(&sb, "- %s %s: %s\n", status, c.Name, c.Detail)
		}
		sb.WriteString("\n")
		if log := sanitize.CleanLog(checkLog, tanksBotCheckLogCapBytes); log != "" {
			sb.WriteString("### Check log (first ")
			fmt.Fprintf(&sb, "%d lines)\n\n```\n", tanksBotCheckLogLines)
			sb.WriteString(firstLines(log, tanksBotCheckLogLines))
			sb.WriteString("\n```\n\n")
		}
	}

	ids, err := s.finishedLadderMatchIDs(ctx, tx, botID, 20)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		sb.WriteString("No matches yet.\n")
		return sb.String(), nil
	}

	sb.WriteString("## Recent matches\n\n")
	for _, id := range ids {
		mv, err := s.matchViewTx(ctx, tx, id)
		if err != nil {
			return "", err
		}
		var mine *MatchPlayerView
		var opponents []string
		for i := range mv.Players {
			p := &mv.Players[i]
			if p.BotID == botID {
				mine = p
			} else {
				opponents = append(opponents, p.Name)
			}
		}
		if mine == nil {
			continue
		}
		place := "?"
		if mine.Place != nil {
			place = fmt.Sprintf("%d", *mine.Place)
		}
		fmt.Fprintf(&sb, "- place %s, %d kills, %d damage, %s, vs %s\n", place, mine.Kills, mine.Damage, mine.Status, strings.Join(opponents, ", "))
	}
	return sb.String(), nil
}

// firstLines returns at most n lines of s, joined back with "\n" and without a trailing newline.
func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
