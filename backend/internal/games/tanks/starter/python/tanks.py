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
