#!/usr/bin/env python3
"""Starter bot: a simple hunter. Edit this file -- improve targeting, add
dodging, use the health pickups, whatever wins more matches.

Run it with: python3 -u bot.py
Play it locally with: arena tanks play . house:hunter house:sniper
"""

import tanks


class Hunter:
    """Charges the nearest living enemy, routing around walls in between,
    and fires once aimed and the line of fire is clear."""

    def __init__(self):
        self.stuck = tanks.StuckWatcher()

    def start(self, msg):
        self.me_id = msg["you"]
        self.start_msg = msg

    def decide(self, msg):
        me = next(t for t in msg["tanks"] if t["id"] == self.me_id)
        if not me["alive"]:
            return {"move": 0, "turn": 0, "turret": 0, "fire": False}

        tick = msg["tick"]

        # Wedged against a wall takes priority over everything else: back
        # off with a turn for a fixed number of ticks, then resume.
        if self.stuck.backing_off() or self.stuck.stuck(tick, me["x"], me["y"]):
            self.stuck.record(tick, me["x"], me["y"], -1.0)
            return {"move": -1.0, "turn": 1.0, "turret": 0, "fire": False}

        target = tanks.nearest_enemy(me, msg["tanks"])
        if target is None:
            self.stuck.record(tick, me["x"], me["y"], 0.0)
            return {"move": 0, "turn": 0, "turret": 0, "fire": False}

        # Steer the hull toward the target, routing around any wall in
        # between instead of driving straight into it -- improve this:
        # back off at close range, or pick a target by lowest HP instead of
        # nearest.
        me_pt = (me["x"], me["y"])
        target_pt = (target["x"], target["y"])
        line_clear = tanks.path_clear(self.start_msg, me_pt, target_pt)
        heading = tanks.steer_around(self.start_msg, me_pt, target_pt, line_clear)
        hull_diff = tanks.angle_diff(heading, me["hull"])
        turn = max(-1.0, min(1.0, 3 * hull_diff))
        dist = tanks.distance(me["x"], me["y"], target["x"], target["y"])
        # Keep closing until both close range AND a clear line of fire --
        # being within 6 units through a wall corner isn't "close enough".
        move = 1.0 if dist > 6 or not line_clear else 0.0

        # Aim the turret straight at the target (not the steering heading),
        # and only fire when the line of fire is clear of walls -- no point
        # spending a reload on cover. Improve this: lead moving targets
        # with tanks.lead_target.
        target_angle = tanks.angle_to(me["x"], me["y"], target["x"], target["y"])
        turret_diff = tanks.angle_diff(target_angle, me["turret"])
        turret = max(-1.0, min(1.0, 4 * turret_diff))
        fire = abs(turret_diff) < 0.08 and line_clear

        self.stuck.record(tick, me["x"], me["y"], move)
        return {"move": move, "turn": turn, "turret": turret, "fire": fire}


if __name__ == "__main__":
    tanks.run(Hunter())
