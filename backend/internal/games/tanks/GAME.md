# Tanks — game rules and bot protocol

tolerance's tanks tournament: your bot is a process. The platform starts
it, sends it the match state once per tick over stdin, and reads your move
back over stdout. Everyone plays with full information — there is no fog of
war.

## 1. Goal

Up to 4 tanks fight on one map. A match ends when the tick limit (1200
ticks, 2 minutes) is reached or at most one tank is left alive. Placement
after that: alive beats dead; among the alive, higher HP wins, then higher
damage dealt; among the dead, whoever died later wins, then higher damage
dealt. Exact ties share a place.

## 2. Coordinate system

- The field is 60 (width, x) by 40 (height, y) units. `(0,0)` is the
  bottom-left corner; `x` grows right, `y` grows up.
- Angles are radians in `(-π, π]`. `0` points along `+x`; positive angles
  turn counter-clockwise.
- Walls (including the field edge) are axis-aligned rectangles:
  `{x, y, w, h}`, `x,y` at the bottom-left corner.

## 3. Rules (engine `tanks/1`)

| Rule | Value |
|---|---|
| Field size | 60 × 40 |
| Tick rate | 10 ticks/s |
| Match length | 1200 ticks (2 minutes) |
| Physics substeps per tick | 4 |
| Tank radius | 1.0 |
| Forward speed | 5 units/s |
| Reverse speed | 3 units/s |
| Hull turn rate | 2.5 rad/s |
| Turret turn rate | 4 rad/s |
| Reload time | 10 ticks |
| Muzzle offset (shell spawn point) | 1.3 from tank centre |
| Shell speed | 24 units/s |
| Shell lifetime | 30 ticks |
| Shell damage | 25 |
| Max HP | 100 |
| Heal pickup amount | +35 HP (capped at max HP) |
| Heal pickup respawn | 150 ticks after being taken |
| Shrinking zone | starts tick 800, radius 37 → radius 6 by tick 1100, then holds |
| Zone damage | 1 HP/tick while outside it |

There is no inertia: your effective speed is `move × max speed` every tick,
not a force. A dead tank's wreck does not collide with anything — it takes
no part in tank-tank or tank-wall pushing, and shells pass through it as if
it were empty ground.

Your own shells never hit you.

## 4. Protocol

One JSON object per line, UTF-8, at most 64 KiB per line. The platform
writes lines to your stdin; you write lines to your stdout. **Never write
anything else to stdout** — put debug output on stderr instead (see §6).

### 4.1 `start` (once, before the first tick)

```json
{"type":"start","you":2,"map":"crossroads",
 "rules":{"width":60,"height":40,"tick_rate":10,"ticks":1200, "...": "..."},
 "walls":[{"x":28,"y":26,"w":4,"h":12}],
 "players":[{"id":0,"name":"house:hunter"},{"id":1,"name":"house:sniper"},{"id":2,"name":"my-tank"}]}
```

`you` is your tank's id — use it to find yourself in every later `tick`'s
`tanks` array. `rules` is the full ruleset table from §3, so you never need
to hardcode the numbers here. Reply:

```json
{"type":"ready"}
```

within **5 seconds** of receiving `start` (this covers your interpreter's
startup time too). If you don't, your tank sits still for the whole match
and your status is `timeout`.

### 4.2 `tick` (once per tick, only while you are alive)

```json
{"type":"tick","tick":17,
 "tanks":[{"id":0,"x":12.3,"y":8.1,"hull":0.52,"turret":0.9,"hp":75,"reload":3,"alive":true}],
 "shells":[{"id":41,"owner":1,"x":20.0,"y":14.2,"vx":-24.0,"vy":0.0}],
 "bonuses":[{"x":30.0,"y":24.0}],
 "zone":{"x":30,"y":20,"r":37}}
```

`bonuses` lists only *active* pickups — one on cooldown after being taken
just isn't there. Reply with your move, echoing the same `tick`:

```json
{"tick":17,"move":1,"turn":0,"turret":-0.5,"fire":true}
```

- `move`, `turn`, `turret` are clamped to `[-1, 1]`.
- `move`: `1` full speed forward, `-1` full speed reverse.
- `turn`: hull turn rate fraction, positive is counter-clockwise.
- `turret`: turret turn rate fraction, same sign convention; the turret
  angle is absolute (not relative to the hull).
- `fire: true` shoots if your reload is `0` this tick; otherwise it's a
  no-op, not an error.

### 4.3 `end` (once, after the match is over)

```json
{"type":"end","place":2,
 "players":[{"slot":0,"place":1,"kills":2,"damage":150,"death_tick":null,"status":"ok"}]}
```

Exit after this — the platform closes your stdin right after sending it.

## 5. Timing and the time budget

- **Ready deadline**: 5 seconds from `start` to `ready`.
- **Per-tick deadline**: 200 ms to answer a `tick`. Miss it and that tick is
  skipped (your tank just doesn't act) — you are not disconnected for one
  slow tick.
- **Time budget**: the first 20 ms of thinking time per tick is free; time
  spent beyond that is paid out of a shared 20-second budget for the whole
  match. Run the budget out and your bot is disconnected for the rest of
  the match (status `timeout`).
- A reply carrying the wrong `tick` (a stale answer to an earlier tick) is
  discarded, same as a missed tick.

## 6. Logs and stray output

**stdout is only for protocol replies.** A very common mistake is a debug
`print(...)` left in before your JSON — that line isn't a JSON object with
a numeric `tick` field, so it's *ignored*, not read as your move, but it
*is* counted. Your move for that tick still counts if the real reply also
arrives in time. More than **1000** such stray lines in one match disable
your bot for the rest of it (status `invalid`); the version check report
tells you to move logging to stderr.

Write logs to **stderr** instead — up to 16 KiB per match, sanitized on the
server, visible only to you (your bot's owner) on the match log page.

## 7. Statuses

| Status | Meaning |
|---|---|
| `ok` | Answered normally for the whole match (or until it died). |
| `crashed` | The process exited before the match ended. |
| `timeout` | Missed the `ready` deadline, or ran out of the time budget. |
| `invalid` | Sent more than 1000 non-command stdout lines. |

In every case your tank stays on the field — it just stops moving from
that point on, so the match doesn't get one-sided by disconnects alone.

## 8. Bot package

A directory with a `bot.json` manifest at its root:

```json
{"name": "my-tank", "language": "python", "entry": "bot.py"}
```

`language` is `python` or `javascript`. The platform runs your bot as
`python3 -u <entry>` or `node <entry>` — `-u` for Python turns off stdout
buffering, which you need for the line-per-message protocol to work at all.

Packed as a `tar.gz`: at most 1 MiB compressed, 4 MiB uncompressed, 200
regular files, no absolute paths or `..`. Only the standard library is
available — the run image is `python:3.12-slim` plus Node 22, no installed
third-party packages. `GAME.md` and `RESULTS.md`, if present in your bot's
folder, are stripped out before packing; they aren't part of the bot.

## 9. Playing locally

The `arena` connector plays matches on your own machine, using the exact
same engine as the server:

```sh
arena tanks new mybot --lang python     # scaffold a starter bot
arena tanks play mybot house:hunter house:sniper --seed 1
arena tanks play mybot house:hunter house:sniper --seed 2
```

Each run prints a results table (place, kills, damage, status, and the
stderr tail of anyone who crashed) and writes a replay file you can open at
`/tanks/replay`. Try a handful of different `--seed` values — a strategy
that only wins on one seed is fragile.

## 10. Joining the tournament

Every uploaded or agent-written version goes through `Qualify` before it can
play in the ladder:

1. **`package`** — the archive is well-formed, `bot.json` parses, and
   `entry` exists.
2. **`starts`** — the bot answers `ready` within 5 seconds.
3. **`stable`** — in a 600-tick trial match against `house:idle` and
   `house:hunter`, it answers at least 95% of the ticks it was alive for,
   and doesn't crash. Stray stdout lines don't fail this check on their
   own, but they show up in the report with a hint to use stderr instead.
4. **`beats_idle`** — it finishes above `house:idle` in that trial match.

All four pass: the version goes `active` and is what plays in the ladder.
Any one fails: the version is `rejected` and your previous active version
(if any) keeps playing.

## 11. Rating

Matches are rated with Weng–Lin (Plackett–Luce), the same idea as
TrueSkill/OpenSkill: every bot has a skill estimate `μ` and an uncertainty
`σ`, both updated from where it placed relative to everyone else in the
match. Starting values are `μ₀ = 25`, `σ₀ = 25/3`; the model's own
parameters are `β = σ₀ / 2` and `κ = 0.0001`. A brand new *version* of an
existing bot doesn't reset its rating, but its uncertainty is bumped back up
to at least `5.0` — a new version is only weak evidence about how it'll
actually do.

The number shown on the leaderboard is:

```
displayed_rating = round(1000 + 40 × (μ − 3σ))
```

— a conservative estimate that starts low and climbs as the bot proves
itself, the same shape TrueSkill's public displays use.
