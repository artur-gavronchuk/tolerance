// Local tanks tournament tooling: scaffold a starter bot (`arena tanks new`),
// play local matches against other bots or the house strategies without any
// server (`arena tanks play`), and upload a bot version to the platform
// (`arena tanks submit`).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"tolerance/internal/games/botpkg"
	"tolerance/internal/games/match"
	"tolerance/internal/games/tanks"
)

// defaultReplayFile is where `arena tanks play` writes the match replay when
// --out isn't given.
const defaultReplayFile = "tanks-replay.json"

// runTanks dispatches `arena tanks <new|play|submit> ...`.
func runTanks(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: arena tanks <new|play|submit>")
	}
	switch args[0] {
	case "new":
		return tanksNew(args[1:], stdout)
	case "play":
		return tanksPlay(args[1:], stdout)
	case "submit":
		return tanksSubmit(args[1:], stdout)
	default:
		return fmt.Errorf("usage: arena tanks <new|play|submit>, got %q", args[0])
	}
}

// tanksNew scaffolds a starter bot in dir: arena tanks new <dir> [--lang python|js].
func tanksNew(args []string, stdout io.Writer) error {
	dir := ""
	lang := "python"

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--lang":
			i++
			if i >= len(args) {
				return errors.New("--lang needs a value")
			}
			lang = args[i]
		default:
			if strings.HasPrefix(a, "-") {
				return fmt.Errorf("unknown flag %q", a)
			}
			if dir != "" {
				return errors.New("usage: arena tanks new <dir> [--lang python|js]")
			}
			dir = a
		}
		i++
	}
	if dir == "" {
		return errors.New("usage: arena tanks new <dir> [--lang python|js]")
	}

	if err := requireEmptyDir(dir); err != nil {
		return err
	}

	files, err := tanks.Starter(lang)
	if err != nil {
		return err
	}
	for rel, data := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return err
		}
	}

	fmt.Fprintf(stdout, "Wrote a %s starter bot to %s\n", lang, dir)
	fmt.Fprintf(stdout, "Next: cd %s && arena tanks play . house:hunter\n", dir)
	return nil
}

// requireEmptyDir requires dir not to exist yet, or to exist and be empty;
// it creates dir (and any parents) in the former case.
func requireEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0o755)
		}
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s already exists and is not empty", dir)
	}
	return nil
}

// tanksPlayOpts is tanksPlay's parsed command line.
type tanksPlayOpts struct {
	bots     []string
	seed     int64
	seedSet  bool
	mapName  string
	ticks    int
	ticksSet bool
	out      string
}

// parseTanksPlayArgs parses `arena tanks play <bot>... [--seed N] [--map NAME] [--ticks N] [--out FILE]`.
// Flags may appear before, after or between the bot arguments.
func parseTanksPlayArgs(args []string) (tanksPlayOpts, error) {
	opts := tanksPlayOpts{out: defaultReplayFile}

	i := 0
	for i < len(args) {
		a := args[i]
		switch a {
		case "--seed":
			i++
			if i >= len(args) {
				return opts, errors.New("--seed needs a value")
			}
			v, err := strconv.ParseInt(args[i], 10, 64)
			if err != nil {
				return opts, fmt.Errorf("--seed: %w", err)
			}
			opts.seed, opts.seedSet = v, true
		case "--map":
			i++
			if i >= len(args) {
				return opts, errors.New("--map needs a value")
			}
			opts.mapName = args[i]
		case "--ticks":
			i++
			if i >= len(args) {
				return opts, errors.New("--ticks needs a value")
			}
			v, err := strconv.Atoi(args[i])
			if err != nil {
				return opts, fmt.Errorf("--ticks: %w", err)
			}
			opts.ticks, opts.ticksSet = v, true
		case "--out":
			i++
			if i >= len(args) {
				return opts, errors.New("--out needs a value")
			}
			opts.out = args[i]
		default:
			if strings.HasPrefix(a, "-") {
				return opts, fmt.Errorf("unknown flag %q", a)
			}
			opts.bots = append(opts.bots, a)
		}
		i++
	}
	return opts, nil
}

// tanksPlay plays one local match: arena tanks play <bot>... [--seed N] [--map NAME] [--ticks N] [--out FILE].
// It returns an error only for a bad bot package or a platform failure (e.g. no python3 on PATH); a bot
// that crashes or times out during a played match is reported in the table, not as an error.
func tanksPlay(args []string, stdout io.Writer) error {
	opts, err := parseTanksPlayArgs(args)
	if err != nil {
		return err
	}
	if n := len(opts.bots); n < 2 || n > 4 {
		return fmt.Errorf("need 2..4 bots, got %d", n)
	}
	if opts.mapName != "" {
		if _, ok := tanks.MapByName(opts.mapName); !ok {
			return fmt.Errorf("unknown map %q; valid maps: %s", opts.mapName, strings.Join(mapNames(), ", "))
		}
	}
	if opts.ticksSet && opts.ticks <= 0 {
		return errors.New("--ticks must be > 0")
	}
	if !opts.seedSet {
		opts.seed = rand.Int64N(1 << 53)
	}

	players, err := buildPlayers(opts.bots)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Seed: %d (reproduce with --seed %d)\n", opts.seed, opts.seed)

	cfg := match.Config{Seed: opts.seed, Map: opts.mapName, Ticks: opts.ticks}
	res, err := match.Run(context.Background(), match.WithHouse(match.ProcessLauncher{}), cfg, players)
	if err != nil {
		return err
	}

	printResultTable(stdout, players, res)

	data, err := json.Marshal(res.Replay)
	if err != nil {
		return fmt.Errorf("encode replay: %w", err)
	}
	if err := os.WriteFile(opts.out, data, 0o644); err != nil {
		return fmt.Errorf("write replay: %w", err)
	}
	fmt.Fprintf(stdout, "Replay: %s — open it at %s/tanks/replay\n", opts.out, siteURL())
	return nil
}

// tanksSubmit packs dir into an archive with botpkg.PackDir — the same validation the server's
// /connector/tanks/versions route runs, so a bad package is rejected locally before any network call — and
// uploads it as a new version of the caller's bot: arena tanks submit <dir>.
func tanksSubmit(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: arena tanks submit <dir>")
	}
	dir := args[0]

	archive, _, err := botpkg.PackDir(dir)
	if err != nil {
		return err
	}

	c, cfg, err := newClient()
	if err != nil {
		return err
	}
	v, err := c.SubmitBot(context.Background(), archive)
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == http.StatusUnauthorized {
			return errors.New("API key rejected; run `arena login` with a fresh key")
		}
		return err
	}

	fmt.Fprintf(stdout, "Uploaded version %d (%s). It plays a check match in a minute: %s/app/tanks\n", v.Number, v.Status, cfg.URL)
	return nil
}

// mapNames lists the built-in map names, in tanks.Maps' fixed order.
func mapNames() []string {
	maps := tanks.Maps()
	names := make([]string, len(maps))
	for i, m := range maps {
		names[i] = m.Name
	}
	return names
}

// siteURL is where the site's replay viewer lives, for the hint printed after a match: config.yaml's url
// when `arena init` has run, otherwise a placeholder. It never requires a key or a network call.
func siteURL() string {
	if cfg, err := loadConfig(); err == nil && cfg.URL != "" {
		return cfg.URL
	}
	return defaultSiteURL
}

// buildPlayers turns each `arena tanks play` argument into a match.Player: "house:<name>" selects a house
// strategy, anything else is a bot directory validated with botpkg.PackDir (the same check the server would
// run on upload, so a bad package is caught locally with the same error text). Duplicate names — two bots
// named the same in their bot.json, or two house bots of the same kind — get "#2", "#3", ... suffixes.
func buildPlayers(bots []string) ([]match.Player, error) {
	players := make([]match.Player, len(bots))
	names := make([]string, len(bots))

	for i, arg := range bots {
		if strat, ok := strings.CutPrefix(arg, "house:"); ok {
			names[i] = "house:" + strat
			players[i] = match.Player{Spec: match.Spec{House: strat}}
			continue
		}
		_, m, err := botpkg.PackDir(arg)
		if err != nil {
			return nil, err
		}
		names[i] = m.Name
		players[i] = match.Player{Spec: match.Spec{Dir: arg, Language: m.Language, Entry: m.Entry}}
	}

	seen := make(map[string]int, len(names))
	for i, name := range names {
		seen[name]++
		if seen[name] > 1 {
			name = fmt.Sprintf("%s#%d", name, seen[name])
		}
		players[i].Name = name
	}
	return players, nil
}

// printResultTable prints the place/name/kills/damage/status/answered table sorted by place, followed by a
// stray-stdout-noise hint for any bot that printed non-command lines and the last 20 lines of stderr for
// every bot whose status isn't "ok".
func printResultTable(stdout io.Writer, players []match.Player, res match.Result) {
	order := make([]int, len(res.Players))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return res.Players[order[a]].Place < res.Players[order[b]].Place
	})

	tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "place\tname\tkills\tdamage\tstatus\tanswered")
	for _, i := range order {
		o := res.Players[i]
		fmt.Fprintf(tw, "%d\t%s\t%d\t%d\t%s\t%d\n", o.Place, players[i].Name, o.Kills, o.Damage, o.Status, o.Answered)
	}
	tw.Flush()

	for _, o := range res.Players {
		if o.Noise > 0 {
			fmt.Fprintf(stdout, "%s printed %d non-command lines to stdout; write logs to stderr\n", players[o.Slot].Name, o.Noise)
		}
	}

	for _, o := range res.Players {
		if o.Status == match.StatusOK {
			continue
		}
		fmt.Fprintf(stdout, "\n--- %s stderr (last 20 lines) ---\n%s\n", players[o.Slot].Name, lastLines(o.Stderr, 20))
	}
}

// lastLines returns at most the last n newline-separated lines of s.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
