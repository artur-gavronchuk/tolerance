package games

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	"tolerance/internal/games/tanks"
	"tolerance/internal/platform/httpx"
	"tolerance/internal/platform/sanitize"
)

// reportBudget is the soft cap on a report's size: it is meant to be pasted into a coding agent next to
// other reports, so the timeline and the log tail are trimmed to fit.
const reportBudget = 6000

// MatchReport renders a plain-text report of one match for the owner of a bot that played in it, meant to
// be pasted into the owner's coding agent. 404 not_found if userID has no bot in the match.
func (s *Service) MatchReport(ctx context.Context, userID, matchID string) (string, error) {
	var mv MatchView
	var mySlot, answered, asked, noise int
	var stderr string
	var replayGz []byte
	err := s.pool.Tx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			SELECT mp.slot, coalesce(mp.answered, 0), coalesce(mp.asked, 0), coalesce(mp.noise, 0), coalesce(mp.stderr_tail, '')
			FROM match_players mp JOIN game_bots gb ON gb.id = mp.bot_id
			WHERE mp.match_id = $1 AND gb.owner_user_id = $2`, matchID, userID).
			Scan(&mySlot, &answered, &asked, &noise, &stderr); err != nil {
			return err
		}
		var err error
		if mv, err = s.matchViewTx(ctx, tx, matchID); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT data FROM match_replays WHERE match_id = $1`, matchID).Scan(&replayGz)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", httpx.NotFound()
	}
	if err != nil {
		return "", err
	}
	var replay *tanks.Replay
	if len(replayGz) > 0 {
		if r, err := tanks.DecodeReplay(replayGz); err == nil && len(r.Frames) > 1 {
			replay = &r
		}
	}
	return buildReport(mv, mySlot, answered, asked, noise, stderr, replay), nil
}

type reportLine struct {
	tick int
	prio int // lower is dropped first when the report is over budget
	text string
}

type observation struct {
	score int
	text  string
}

type span struct{ from, to int }

func (s span) ticks() int { return s.to - s.from + 1 }

func cleanName(n string) string { return sanitize.CleanText(n, 40) }

func fmtPos(x, y float64) string { return fmt.Sprintf("(%.1f, %.1f)", x, y) }

func fmtTicks(a, b int) string {
	if a == b {
		return fmt.Sprintf("t=%d", a)
	}
	return fmt.Sprintf("t=%d-%d", a, b)
}

func deg(rad float64) float64 { return math.Abs(rad) * 180 / math.Pi }

func angDiff(a, b float64) float64 {
	d := math.Mod(a-b, 2*math.Pi)
	if d > math.Pi {
		d -= 2 * math.Pi
	} else if d < -math.Pi {
		d += 2 * math.Pi
	}
	return d
}

// wallGap returns how far the edge of a tank-radius circle at (x, y) is from the nearest wall or field
// edge, and a description of that obstacle.
func wallGap(rp *tanks.Replay, x, y float64) (float64, string) {
	r := rp.Rules
	best := math.Inf(1)
	what := ""
	edges := []struct {
		d    float64
		name string
	}{{x, "west edge"}, {r.Width - x, "east edge"}, {y, "south edge"}, {r.Height - y, "north edge"}}
	for _, e := range edges {
		if e.d-r.TankRadius < best {
			best, what = e.d-r.TankRadius, "the "+e.name+" of the field"
		}
	}
	for _, w := range rp.Walls {
		cx := math.Max(w.X, math.Min(x, w.X+w.W))
		cy := math.Max(w.Y, math.Min(y, w.Y+w.H))
		if d := math.Hypot(x-cx, y-cy) - r.TankRadius; d < best {
			best = d
			what = fmt.Sprintf("the wall block x %.0f..%.0f, y %.0f..%.0f", w.X, w.X+w.W, w.Y, w.Y+w.H)
		}
	}
	return best, what
}

// blocked reports whether a wall lies on the straight line between two points.
func blocked(rp *tanks.Replay, x1, y1, x2, y2 float64) bool {
	d := math.Hypot(x2-x1, y2-y1)
	steps := int(d / 0.4)
	for i := 1; i < steps; i++ {
		f := float64(i) / float64(steps)
		x, y := x1+(x2-x1)*f, y1+(y2-y1)*f
		for _, w := range rp.Walls {
			if x >= w.X && x <= w.X+w.W && y >= w.Y && y <= w.Y+w.H {
				return true
			}
		}
	}
	return false
}

// groupTicks merges ascending ticks into spans where consecutive ticks are at most gap apart.
func groupTicks(ticks []int, gap int) []span {
	var out []span
	for _, t := range ticks {
		if n := len(out); n > 0 && t-out[n-1].to <= gap {
			out[n-1].to = t
			continue
		}
		out = append(out, span{t, t})
	}
	return out
}

// buildReport is the pure rendering of MatchReport: everything it knows comes from the arguments.
func buildReport(mv MatchView, me, answered, asked, noise int, stderr string, rp *tanks.Replay) string {
	var mine *MatchPlayerView
	for i := range mv.Players {
		if mv.Players[i].Slot == me {
			mine = &mv.Players[i]
		}
	}
	if mine == nil {
		return "No data for this bot in this match.\n"
	}
	name := func(slot int) string {
		for _, p := range mv.Players {
			if p.Slot == slot {
				if slot == me {
					return "you"
				}
				return cleanName(p.Name)
			}
		}
		return fmt.Sprintf("slot %d", slot)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Tanks match report: %s v%d (%s match)\n\n", cleanName(mine.Name), mine.Version, mv.Kind)
	played := mv.Ticks
	if rp != nil {
		played = len(rp.Frames) - 1
	}
	fmt.Fprintf(&b, "Match %s, map %s, seed %d, %d ticks played (10 ticks = 1 s), status %s.\n", mv.ID, mv.Map, mv.Seed, played, mv.Status)
	if rp != nil {
		fmt.Fprintf(&b, "Field %.0fx%.0f, (0,0) bottom-left. Zone: radius %.0f, shrinks from t=%d to radius %.0f at t=%d, %d HP/tick damage outside it.\n",
			rp.Rules.Width, rp.Rules.Height, rp.Rules.ZoneStartRadius, rp.Rules.ZoneStartTick, rp.Rules.ZoneEndRadius, rp.Rules.ZoneEndTick, rp.Rules.ZoneDamagePerTick)
	}
	if mv.Status != "finished" {
		fmt.Fprintf(&b, "\nThis match has not finished (status %s); there is no result yet.\n", mv.Status)
		return b.String()
	}

	// Result.
	b.WriteString("\n## Result\n")
	ranked := append([]MatchPlayerView(nil), mv.Players...)
	sort.SliceStable(ranked, func(i, j int) bool {
		pi, pj := 99, 99
		if ranked[i].Place != nil {
			pi = *ranked[i].Place
		}
		if ranked[j].Place != nil {
			pj = *ranked[j].Place
		}
		return pi < pj
	})
	for _, p := range ranked {
		place := "?"
		if p.Place != nil {
			place = fmt.Sprint(*p.Place)
		}
		end := "alive at the end"
		if p.DeathTick != nil {
			end = fmt.Sprintf("died t=%d", *p.DeathTick)
		}
		hp := ""
		if rp != nil && p.Slot < len(rp.Frames[len(rp.Frames)-1].K) && p.DeathTick == nil {
			hp = fmt.Sprintf(" with %.0f HP", rp.Frames[len(rp.Frames)-1].K[p.Slot][4])
		}
		label := name(p.Slot)
		if p.Slot == me {
			label = "YOU (" + cleanName(p.Name) + " v" + fmt.Sprint(p.Version) + ")"
		}
		fmt.Fprintf(&b, "%s. %s: %d kills, %d damage dealt, %s%s, status %s\n", place, label, p.Kills, p.Damage, end, hp, p.Status)
	}
	if mine.RatingBefore != nil && mine.RatingAfter != nil {
		fmt.Fprintf(&b, "Your rating: %d -> %d.\n", *mine.RatingBefore, *mine.RatingAfter)
	}
	b.WriteString("Placement rule: alive beats dead; alive ties go to higher HP, then damage dealt; among the dead, who died later wins.\n")
	fmt.Fprintf(&b, "Your bot: status %s, answered %d of %d ticks in time", mine.Status, answered, asked)
	if noise > 0 {
		fmt.Fprintf(&b, ", %d stdout lines that were not commands (stdout must carry only JSON commands)", noise)
	}
	b.WriteString(".\n")

	var obs []observation
	switch mine.Status {
	case "timeout":
		obs = append(obs, observation{100, fmt.Sprintf("Your bot timed out (answered %d of %d ticks, or never sent ready within 5 s). A missed tick means your tank does nothing that tick. Make each tick's work well under 200 ms and answer the start message with ready immediately.", answered, asked)})
	case "crashed":
		obs = append(obs, observation{100, "Your bot process crashed or closed stdout. See the stderr tail below; a crashed tank sits still for the rest of the match."})
	case "invalid":
		obs = append(obs, observation{100, fmt.Sprintf("Your bot was disabled for writing too much to stdout that was not a command (%d stray lines). Send debug output to stderr only.", noise)})
	}

	var timeline []reportLine
	if rp == nil {
		b.WriteString("\nThe replay is no longer stored (replays are kept 3 days), so there is no timeline; the result and log below are all that is left.\n")
	} else {
		tl, o := analyseTimeline(rp, mv, me, asked, answered, mine, name)
		timeline = tl
		obs = append(obs, o...)
	}

	if len(obs) == 0 {
		obs = append(obs, observation{1, "Nothing stood out in this match."})
	}
	sort.SliceStable(obs, func(i, j int) bool { return obs[i].score > obs[j].score })
	if len(obs) > 5 {
		obs = obs[:5]
	}
	b.WriteString("\n## Observations\n")
	for _, o := range obs {
		b.WriteString("- " + o.text + "\n")
	}

	tail := tailLines(sanitize.CleanLog(stderr, 8<<10), 25, 1200)
	logBlock := "\n## Your bot's stderr (last lines)\n"
	if strings.TrimSpace(tail) == "" {
		logBlock += "(empty)\n"
	} else {
		logBlock += "```\n" + tail + "\n```\n"
	}

	// Fit the timeline into what is left of the budget: drop the least important lines first.
	room := reportBudget - b.Len() - len(logBlock) - 60
	kept := fitTimeline(timeline, room)
	if rp != nil {
		b.WriteString("\n## Timeline (your tank; tick = the state your bot saw when it decided)\n")
		for _, l := range kept {
			b.WriteString(l.text + "\n")
		}
		if len(kept) < len(timeline) {
			fmt.Fprintf(&b, "(%d minor lines omitted)\n", len(timeline)-len(kept))
		}
	}
	b.WriteString(logBlock)
	return b.String()
}

func fitTimeline(lines []reportLine, room int) []reportLine {
	size := func(ls []reportLine) int {
		n := 0
		for _, l := range ls {
			n += len(l.text) + 1
		}
		return n
	}
	keep := append([]reportLine(nil), lines...)
	for size(keep) > room && len(keep) > 0 {
		// drop the lowest-priority line, latest first
		idx := 0
		for i, l := range keep {
			if l.prio < keep[idx].prio || (l.prio == keep[idx].prio && l.tick >= keep[idx].tick) {
				idx = i
			}
		}
		keep = append(keep[:idx], keep[idx+1:]...)
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].tick < keep[j].tick })
	return keep
}

func tailLines(s string, maxLines, maxBytes int) string {
	s = strings.TrimRight(s, "\n ")
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	s = strings.Join(lines, "\n")
	if len(s) > maxBytes {
		s = "..." + strings.ToValidUTF8(s[len(s)-maxBytes:], "")
	}
	return s
}

// analyseTimeline derives the timeline lines and the candidate observations of slot me from the replay.
func analyseTimeline(rp *tanks.Replay, mv MatchView, me, asked, answered int, mine *MatchPlayerView, name func(int) string) ([]reportLine, []observation) {
	rules := rp.Rules
	frames := rp.Frames
	n := len(frames) - 1
	if me >= len(frames[0].K) {
		return nil, nil
	}
	pos := func(t, slot int) (float64, float64) { k := frames[t].K[slot]; return k[0], k[1] }
	alive := func(t, slot int) bool { return frames[t].K[slot][6] == 1 }
	cx, cy := rules.Width/2, rules.Height/2

	var lines []reportLine
	var obs []observation
	add := func(tick, prio int, f string, a ...any) {
		lines = append(lines, reportLine{tick, prio, fmt.Sprintf(f, a...)})
	}

	sx, sy := pos(0, me)
	add(0, 9, "t=0 spawn at %s, HP %d.", fmtPos(sx, sy), rules.MaxHP)

	// Events of this tank.
	var shotTicks, hitDealt []int
	hitsDealtDmg := 0
	takenBy := map[int][]int{} // attacker slot -> ticks
	takenDmg := map[int]int{}
	takenCount := 0
	var healTicks []int
	deathTick, killer := -1, -2
	for _, e := range rp.Events {
		switch e.E {
		case "shot":
			if e.A == me {
				shotTicks = append(shotTicks, e.T)
			}
		case "hit":
			d := 0
			if e.D != nil {
				d = *e.D
			}
			if e.A == me {
				hitDealt = append(hitDealt, e.T)
				hitsDealtDmg += d
			}
			if e.B != nil && *e.B == me {
				takenBy[e.A] = append(takenBy[e.A], e.T)
				takenDmg[e.A] += d
				takenCount++
			}
		case "heal":
			if e.A == me {
				healTicks = append(healTicks, e.T)
			}
		case "kill":
			if e.B != nil && *e.B == me {
				deathTick, killer = e.T, e.A
			}
		}
	}

	// Outside the zone (a tank is checked after its move, against that tick's radius).
	var outside []int
	for t := 0; t < n; t++ {
		if !alive(t, me) {
			break
		}
		x, y := pos(t+1, me)
		if math.Hypot(x-cx, y-cy) > frames[t+1].Z {
			outside = append(outside, t)
		}
	}
	zoneDmg := len(outside) * rules.ZoneDamagePerTick
	for _, sp := range groupTicks(outside, 2) {
		x, y := pos(sp.to+1, me)
		add(sp.from, 5, "%s outside the zone for %d ticks (%d HP lost), ending at %s, %.1f from the centre (zone radius there %.1f).",
			fmtTicks(sp.from, sp.to), sp.ticks(), sp.ticks()*rules.ZoneDamagePerTick, fmtPos(x, y), math.Hypot(x-cx, y-cy), frames[sp.to+1].Z)
	}

	// Hits taken, grouped by attacker.
	var attackers []int
	for a := range takenBy {
		attackers = append(attackers, a)
	}
	sort.Ints(attackers)
	for _, a := range attackers {
		for _, sp := range groupTicks(takenBy[a], 20) {
			dmg, cnt := 0, 0
			for _, e := range rp.Events {
				if e.E == "hit" && e.A == a && e.B != nil && *e.B == me && e.T >= sp.from && e.T <= sp.to {
					cnt++
					if e.D != nil {
						dmg += *e.D
					}
				}
			}
			x, y := pos(sp.from, me)
			dist := ""
			if a >= 0 && a < len(frames[0].K) {
				ax, ay := pos(sp.from, a)
				dist = fmt.Sprintf(", attacker %.0f units away", math.Hypot(ax-x, ay-y))
			}
			add(sp.from, 8, "%s took %d damage from %s (%d hit%s), you were at %s%s.", fmtTicks(sp.from, sp.to), dmg, name(a), cnt, plural(cnt), fmtPos(x, y), dist)
		}
	}

	// Shots: bursts, accuracy, aim quality.
	badAim, noLOS, aimed := 0, 0, 0
	for _, t := range shotTicks {
		var best = -1
		bd := math.Inf(1)
		x, y := pos(t, me)
		for s := range frames[t].K {
			if s == me || !alive(t, s) {
				continue
			}
			ex, ey := pos(t, s)
			if d := math.Hypot(ex-x, ey-y); d < bd {
				bd, best = d, s
			}
		}
		if best < 0 {
			continue
		}
		aimed++
		ex, ey := pos(t, best)
		if blocked(rp, x, y, ex, ey) {
			noLOS++
			continue
		}
		bearing := math.Atan2(ey-y, ex-x)
		tol := math.Atan2(rules.TankRadius, bd) + 0.1
		if math.Abs(angDiff(frames[t].K[me][3], bearing)) > tol+0.17 {
			badAim++
		}
	}
	for _, sp := range groupTicks(shotTicks, 12) {
		cnt := 0
		for _, t := range shotTicks {
			if t >= sp.from && t <= sp.to {
				cnt++
			}
		}
		hits := 0
		for _, t := range hitDealt {
			if t >= sp.from && t <= sp.to+rules.ShellLifetimeTicks {
				hits++
			}
		}
		x, y := pos(sp.from, me)
		add(sp.from, 4, "%s fired %d shot%s (you were near %s at the start), %d hit%s.", fmtTicks(sp.from, sp.to), cnt, plural(cnt), fmtPos(x, y), hits, plural(hits))
	}
	for _, sp := range groupTicks(hitDealt, 20) {
		dmg, cnt := 0, 0
		var victim = -1
		for _, e := range rp.Events {
			if e.E == "hit" && e.A == me && e.T >= sp.from && e.T <= sp.to && e.B != nil {
				cnt++
				victim = *e.B
				if e.D != nil {
					dmg += *e.D
				}
			}
		}
		add(sp.from, 7, "%s you dealt %d damage to %s (%d hit%s).", fmtTicks(sp.from, sp.to), dmg, name(victim), cnt, plural(cnt))
	}

	for _, t := range healTicks {
		x, y := pos(t+1, me)
		add(t, 5, "t=%d picked up a heal bonus at %s.", t, fmtPos(x, y))
	}
	for _, e := range rp.Events {
		if e.E == "kill" && e.A == me && e.B != nil {
			add(e.T, 8, "t=%d you killed %s.", e.T, name(*e.B))
		}
	}

	// Not moving: anchor-based, so wiggling in place counts as stuck.
	type stuck struct {
		span
		x, y   float64
		where  string
		near   bool
		turned float64
		shots  int
	}
	var stucks []stuck
	end := n
	if deathTick >= 0 && deathTick < end {
		end = deathTick + 1
	}
	for a := 0; a < end; {
		ax, ay := pos(a, me)
		z := a
		for z+1 < end {
			x, y := pos(z+1, me)
			if math.Hypot(x-ax, y-ay) >= 1.5 {
				break
			}
			z++
		}
		if z-a+1 >= 20 {
			ax, ay = pos(z, me) // describe where the tank ended up, not where it started
			gap, what := wallGap(rp, ax, ay)
			turned := 0.0
			for t := a; t < z; t++ {
				turned += math.Abs(angDiff(frames[t+1].K[me][2], frames[t].K[me][2]))
			}
			sh := 0
			for _, t := range shotTicks {
				if t >= a && t <= z {
					sh++
				}
			}
			stucks = append(stucks, stuck{span{a, z}, ax, ay, what, gap < 0.4, turned * 180 / math.Pi, sh})
			a = z + 1
		} else {
			a++
		}
	}
	wallTicks, stillTicks := 0, 0
	var worstWall *stuck
	for i := range stucks {
		s := &stucks[i]
		stillTicks += s.ticks()
		if s.near {
			wallTicks += s.ticks()
			if worstWall == nil || s.ticks() > worstWall.ticks() {
				worstWall = s
			}
			add(s.from, 9, "%s stuck at %s for %d ticks, pressed against %s; hull turned %.0f deg in total, %d shots fired.",
				fmtTicks(s.from, s.to), fmtPos(s.x, s.y), s.ticks(), s.where, s.turned, s.shots)
		} else {
			add(s.from, 6, "%s barely moved (stayed within 1.5 units of %s) for %d ticks, hull turned %.0f deg, %d shots fired.",
				fmtTicks(s.from, s.to), fmtPos(s.x, s.y), s.ticks(), s.turned, s.shots)
		}
	}

	// Death.
	if deathTick >= 0 {
		x, y := pos(deathTick+1, me)
		by := "the shrinking zone"
		if killer >= 0 {
			by = name(killer)
		}
		add(deathTick, 10, "t=%d you died at %s, killed by %s.", deathTick, fmtPos(x, y), by)
	} else {
		add(n, 10, "t=%d match ended with you alive, %.0f HP.", n, frames[n].K[me][4])
	}

	// Distance travelled.
	travelled := 0.0
	for t := 0; t < end && t < n; t++ {
		x0, y0 := pos(t, me)
		x1, y1 := pos(t+1, me)
		travelled += math.Hypot(x1-x0, y1-y0)
	}
	aliveTicks := end
	fullSpeed := float64(aliveTicks) * rules.TankSpeed / float64(rules.TickRate)

	// Observations.
	shots, hits := len(shotTicks), len(hitDealt)
	if shots == 0 {
		obs = append(obs, observation{95, fmt.Sprintf("You never fired in %d ticks alive. Set \"fire\": true once the turret points at an enemy; fire is a no-op while reload > 0, so sending it every tick is fine.", aliveTicks)})
	} else {
		rate := hits * 100 / shots
		first := shotTicks[0]
		if shots >= 5 && rate < 30 {
			obs = append(obs, observation{80 - rate/2, fmt.Sprintf("You hit %d of %d shots (%d%%). Shell speed is %.0f units/s, so lead moving targets and only fire when the turret is on the target.", hits, shots, rate, rules.ShellSpeed)})
		} else {
			obs = append(obs, observation{20, fmt.Sprintf("You hit %d of %d shots (%d%%), %d damage dealt.", hits, shots, rate, hitsDealtDmg)})
		}
		if aimed > 0 && badAim*100/aimed >= 30 && badAim >= 3 {
			obs = append(obs, observation{75, fmt.Sprintf("%d of %d shots were fired with the turret pointing more than ~15 degrees away from the nearest enemy. Turn the turret first, fire when the angle error is small.", badAim, aimed)})
		}
		if noLOS >= 3 && noLOS*100/shots >= 20 {
			obs = append(obs, observation{72, fmt.Sprintf("%d of %d shots were fired with a wall between you and the nearest enemy; shells stop at walls. Check line of sight before firing.", noLOS, shots)})
		}
		if first > 100 {
			obs = append(obs, observation{40, fmt.Sprintf("Your first shot came only at t=%d.", first)})
		}
	}
	if mine.Status == "ok" && asked-answered >= 3 {
		obs = append(obs, observation{78, fmt.Sprintf("Your bot missed the 200 ms per-tick deadline on %d of %d ticks (each miss = a tick of doing nothing). Keep per-tick work light and never block on stdin/stdout.", asked-answered, asked)})
	}
	if wallTicks >= 30 && worstWall != nil {
		obs = append(obs, observation{88, fmt.Sprintf("You spent %d ticks pressed against %s around %s (longest stretch %s). Detect that your position stops changing and back up or turn away instead of driving into it.", wallTicks, worstWall.where, fmtPos(worstWall.x, worstWall.y), fmtTicks(worstWall.from, worstWall.to))})
	} else if stillTicks >= 80 {
		obs = append(obs, observation{50, fmt.Sprintf("You barely moved for %d of %d ticks alive; a tank that stands still is easy to hit.", stillTicks, aliveTicks)})
	}
	if deathTick >= 0 && killer == -1 {
		x, y := pos(deathTick+1, me)
		obs = append(obs, observation{92, fmt.Sprintf("You died in the shrinking zone at t=%d, at %s (%.1f from the centre). You were outside it for %d ticks. Start heading to the centre (%.0f, %.0f) before the zone starts shrinking at t=%d.", deathTick, fmtPos(x, y), math.Hypot(x-cx, y-cy), len(outside), cx, cy, rules.ZoneStartTick)})
	} else if zoneDmg >= 10 {
		obs = append(obs, observation{60, fmt.Sprintf("You lost %d HP to the shrinking zone (outside it for %d ticks). Stay inside the circle around (%.0f, %.0f) from t=%d on.", zoneDmg, len(outside), cx, cy, rules.ZoneStartTick)})
	}
	if deathTick >= 0 && killer >= 0 {
		obs = append(obs, observation{70, fmt.Sprintf("You were killed by %s at t=%d after taking %d damage from it, while you dealt %d damage in total.", name(killer), deathTick, takenDmg[killer], hitsDealtDmg)})
	}
	if takenCount > 0 && hits == 0 {
		obs = append(obs, observation{65, fmt.Sprintf("You took %d hits and landed none.", takenCount)})
	}
	if deathTick < 0 && mine.Place != nil && *mine.Place > 1 {
		obs = append(obs, observation{55, fmt.Sprintf("You survived but placed %d: alive tanks are ranked by HP, then damage dealt. You ended with %.0f HP and dealt %d damage; deal more damage and avoid being hit.", *mine.Place, frames[n].K[me][4], mine.Damage)})
	}
	if mine.Place != nil && *mine.Place == 1 {
		obs = append(obs, observation{30, "You won this match."})
	}
	if aliveTicks >= 100 && travelled < fullSpeed*0.15 && len(stucks) == 0 {
		obs = append(obs, observation{45, fmt.Sprintf("You travelled only %.0f units in %d ticks alive (full speed would be about %.0f).", travelled, aliveTicks, fullSpeed)})
	}
	if len(healTicks) == 0 && takenCount >= 2 {
		obs = append(obs, observation{25, "You never picked up a heal bonus (+35 HP); the pickups sit at fixed spots and respawn after 150 ticks."})
	}
	return lines, obs
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
