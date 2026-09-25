#!/usr/bin/env python3
"""Starter bot: a simple hunter. Edit this file -- improve targeting, add
dodging, use the health pickups, whatever wins more matches.

Run it with: python3 -u bot.py
Play it locally with: arena tanks play . house:hunter house:sniper
"""

import tanks


class Hunter:
    """Charges the nearest living enemy and fires once aimed."""

    def start(self, msg):
        self.me_id = msg["you"]

    def decide(self, msg):
        me = next(t for t in msg["tanks"] if t["id"] == self.me_id)
        if not me["alive"]:
            return {"move": 0, "turn": 0, "turret": 0, "fire": False}

        target = tanks.nearest_enemy(me, msg["tanks"])
        if target is None:
            return {"move": 0, "turn": 0, "turret": 0, "fire": False}

        # Turn the hull toward the target and drive in, unless we're
        # already close -- improve this: back off at close range, or pick
        # a target by lowest HP instead of nearest.
        target_angle = tanks.angle_to(me["x"], me["y"], target["x"], target["y"])
        hull_diff = tanks.angle_diff(target_angle, me["hull"])
        turn = max(-1.0, min(1.0, 3 * hull_diff))
        dist = tanks.distance(me["x"], me["y"], target["x"], target["y"])
        move = 1.0 if dist > 6 else 0.0

        # Aim the turret straight at the target -- improve this: lead
        # moving targets with tanks.lead_target.
        turret_diff = tanks.angle_diff(target_angle, me["turret"])
        turret = max(-1.0, min(1.0, 4 * turret_diff))
        fire = abs(turret_diff) < 0.08

        return {"move": move, "turn": turn, "turret": turret, "fire": fire}


if __name__ == "__main__":
    tanks.run(Hunter())
