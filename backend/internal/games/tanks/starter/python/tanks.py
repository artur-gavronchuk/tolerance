"""tanks.py -- protocol and helper library for tanks starter bots.

Import this from your bot and call run(your_strategy). See bot.py for an
example strategy and GAME.md for the rules and protocol this wraps.

No third-party dependencies -- standard library only, as required by the
package rules.
"""

import json
import math
import sys


def log(*args):
    """Print a debug message to stderr. Never print to stdout except
    through this module's run() loop -- stray stdout lines are ignored by
    the platform but count against your bot's stray-output limit."""
    print(*args, file=sys.stderr, flush=True)


def angle_to(x, y, tx, ty):
    """Absolute angle from (x, y) to (tx, ty), in (-pi, pi]."""
    return math.atan2(ty - y, tx - x)


def angle_diff(a, b):
    """Signed shortest difference a - b, normalized to (-pi, pi]."""
    d = (a - b + math.pi) % (2 * math.pi) - math.pi
    if d <= -math.pi:
        d += 2 * math.pi
    return d


def distance(x, y, tx, ty):
    return math.hypot(tx - x, ty - y)


def nearest_enemy(me, tanks):
    """The closest living tank other than `me` (a tank dict), or None."""
    best = None
    best_d = None
    for t in tanks:
        if t["id"] == me["id"] or not t["alive"]:
            continue
        d = distance(me["x"], me["y"], t["x"], t["y"])
        if best_d is None or d < best_d:
            best, best_d = t, d
    return best


def lead_target(shooter, target, shell_speed, target_vx=0.0, target_vy=0.0):
    """Aim point that leads a moving target: solves for where a shell fired
    now at shell_speed meets a target moving at (target_vx, target_vy) from
    its current position. Falls back to the target's current position if
    there is no solution (e.g. it's outrunning the shell)."""
    dx = target["x"] - shooter["x"]
    dy = target["y"] - shooter["y"]

    a = target_vx * target_vx + target_vy * target_vy - shell_speed * shell_speed
    b = 2 * (dx * target_vx + dy * target_vy)
    c = dx * dx + dy * dy

    t = None
    if abs(a) < 1e-9:
        if abs(b) > 1e-9:
            candidate = -c / b
            if candidate > 0:
                t = candidate
    else:
        disc = b * b - 4 * a * c
        if disc >= 0:
            sq = math.sqrt(disc)
            candidates = [x for x in ((-b + sq) / (2 * a), (-b - sq) / (2 * a)) if x > 0]
            if candidates:
                t = min(candidates)

    if t is None:
        return target["x"], target["y"]
    return target["x"] + target_vx * t, target["y"] + target_vy * t


# Wall avoidance. bunkers's per-spawn L-shaped walls trap a bot that only
# ever turns straight toward its target -- path_clear and steer_around give
# you a cheap way to check a line against the map and route around an
# obstacle instead of driving into it.

STEER_MARGIN = 0.4  # extra clearance beyond the tank's own radius
STEER_LOOKAHEAD = 6.0  # how far ahead a candidate heading is checked, in units
STEER_OFFSETS_DEG = (30, 60, 90, 120)  # tried in order, nearest first


def _segment_enters_rect(x1, y1, x2, y2, rect, pad):
    """True if the segment (x1,y1)-(x2,y2) enters rect (a
    {"x","y","w","h"} dict) inflated by pad on every side. Liang-Barsky
    segment-vs-AABB clipping: walk the segment's parametric range [0, 1]
    through each of the rect's four half-plane constraints -- it only enters
    the rect if a non-empty sub-range survives all four."""
    rx0 = rect["x"] - pad
    ry0 = rect["y"] - pad
    rx1 = rect["x"] + rect["w"] + pad
    ry1 = rect["y"] + rect["h"] + pad
    dx = x2 - x1
    dy = y2 - y1
    t0, t1 = 0.0, 1.0
    for p, q in ((-dx, x1 - rx0), (dx, rx1 - x1), (-dy, y1 - ry0), (dy, ry1 - y1)):
        if abs(p) < 1e-9:
            if q < 0:
                return False  # parallel to this edge and outside it: never enters
            continue
        t = q / p
        if p < 0:
            if t > t1:
                return False
            t0 = max(t0, t)
        else:
            if t < t0:
                return False
            t1 = min(t1, t)
    return t0 <= t1  # non-empty surviving interval: the segment enters the rect


def _dist_to_rect(x, y, rect):
    """Distance from (x, y) to rect's boundary, or 0 if (x, y) is inside it."""
    cx = max(rect["x"], min(x, rect["x"] + rect["w"]))
    cy = max(rect["y"], min(y, rect["y"] + rect["h"]))
    return distance(x, y, cx, cy)


def _segment_clears_rect(x1, y1, x2, y2, rect, pad):
    """True if the segment (x1,y1)-(x2,y2) stays clear of rect inflated by
    pad on every side. A tank resting against a wall sits at distance
    tank_radius from its face -- inside the padded margin (tank_radius +
    STEER_MARGIN) the engine's own collision push-out leaves it at. Treating
    that as "blocked" the ordinary way would call every heading out of a
    wall it's already touching blocked too, direct heading included, so:
    when the segment starts inside rect's padded margin, this rect only
    blocks a heading that either actually enters the solid (unpadded) rect,
    or ends up no farther from it than the start -- any heading that gets
    strictly farther away is let through. A segment starting outside the
    padded margin is checked the ordinary way."""
    start_dist = _dist_to_rect(x1, y1, rect)
    if pad > 0 and start_dist < pad:
        if _segment_enters_rect(x1, y1, x2, y2, rect, 0):
            return False  # actually enters the solid wall: always blocks
        if _dist_to_rect(x2, y2, rect) >= start_dist:
            return True  # already touching this wall, and headed no closer: not blocked by it
    return not _segment_enters_rect(x1, y1, x2, y2, rect, pad)


def path_clear(start, from_point, to_point):
    """True if a straight line from from_point to to_point (each an (x, y)
    pair) stays clear of every wall in `start` (the start message), each
    inflated by the tank's own radius plus a small margin. Use this before
    committing to a heading, and before firing, so you don't drive -- or
    shoot -- straight into cover."""
    pad = start["rules"]["tank_radius"] + STEER_MARGIN
    x1, y1 = from_point
    x2, y2 = to_point
    return all(_segment_clears_rect(x1, y1, x2, y2, wall, pad) for wall in start["walls"])


def steer_around(start, me, target, direct_clear=None):
    """Heading (radians) to drive from me toward target (each an (x, y)
    pair), routing around walls: the direct heading if that line is clear,
    otherwise the first clear heading among the +-30/60/90/120 degree
    offsets in STEER_OFFSETS_DEG, checked STEER_LOOKAHEAD units ahead and
    preferring whichever side of a given offset closes more distance to
    target. Falls back to the direct heading if every offset is blocked, so
    a boxed-in tank still pushes toward its target rather than freezing.
    Pass direct_clear if you already called path_clear(start, me, target)
    for something else (e.g. gating fire), to skip recomputing it here."""
    mx, my = me
    tx, ty = target
    direct = angle_to(mx, my, tx, ty)
    if direct_clear is None:
        direct_clear = path_clear(start, me, target)
    if direct_clear:
        return direct

    for deg in STEER_OFFSETS_DEG:
        best, best_gain = direct, None
        for sign in (1, -1):
            heading = direct + sign * math.radians(deg)
            look = (mx + STEER_LOOKAHEAD * math.cos(heading), my + STEER_LOOKAHEAD * math.sin(heading))
            if not path_clear(start, me, look):
                continue
            gain = distance(mx, my, tx, ty) - distance(look[0], look[1], tx, ty)
            if best_gain is None or gain > best_gain:
                best, best_gain = heading, gain
        if best_gain is not None:
            return best
    return direct


class StuckWatcher:
    """Detects a tank that was told to move but barely moved over the last
    `window` ticks -- wedged against a wall -- and then walks the caller
    through a fixed reverse-and-turn recovery for `backoff_ticks` ticks.

    Call backing_off() and stuck() in that order at the top of decide(); if
    either returns True, issue {"move": -1, "turn": 1, ...} for this tick
    instead of your normal plan. Either way, call record() once per tick
    with the position you're at and the move you issued, so the next tick's
    check has this tick's sample to compare against."""

    def __init__(self, window=8, move_threshold=0.2, backoff_ticks=6):
        self.window = window
        self.move_threshold = move_threshold
        self.backoff_ticks = backoff_ticks
        self._history = []  # (tick, x, y), oldest first, trimmed to `window` samples
        self._moves = []  # move issued each of the last `window` ticks
        self._backoff_left = 0

    def backing_off(self):
        """True while still inside the post-stuck recovery window; consumes
        one tick of it when so."""
        if self._backoff_left <= 0:
            return False
        self._backoff_left -= 1
        return True

    def stuck(self, tick, x, y):
        """True if, over the window recorded so far, the tank was told to
        move but barely did; if so, starts the recovery window. `_history`
        holds exactly `window` samples once warmed up (the ticks
        immediately before this one), so its oldest entry is `window` ticks
        behind the current tick -- the comparison this method needs."""
        if len(self._history) < self.window:
            return False
        oldest_tick, ox, oy = self._history[0]
        if tick - oldest_tick != self.window:
            return False
        if distance(ox, oy, x, y) > self.move_threshold:
            return False
        if not any(m != 0 for m in self._moves):
            return False
        self._backoff_left = self.backoff_ticks - 1
        return True

    def record(self, tick, x, y, move):
        """Append this tick's own position and issued move to the sliding
        window backing_off() and stuck() use."""
        self._history.append((tick, x, y))
        self._moves.append(move)
        if len(self._history) > self.window:
            self._history = self._history[-self.window:]
        if len(self._moves) > self.window:
            self._moves = self._moves[-self.window:]


def run(strategy):
    """Read the protocol from stdin and drive `strategy` until the match
    ends or stdin closes. `strategy` needs a `decide(tick_msg) -> dict`
    method; an optional `start(start_msg)` method is called once before the
    first tick. Unknown message types are ignored, not an error."""
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except ValueError:
            continue
        if not isinstance(msg, dict):
            continue

        kind = msg.get("type")
        if kind == "start":
            if hasattr(strategy, "start"):
                strategy.start(msg)
            print(json.dumps({"type": "ready"}), flush=True)
        elif kind == "tick":
            reply = strategy.decide(msg)
            reply["tick"] = msg["tick"]
            print(json.dumps(reply), flush=True)
        elif kind == "end":
            return
