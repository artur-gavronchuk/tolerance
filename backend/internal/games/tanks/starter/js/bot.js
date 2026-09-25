'use strict';
// Starter bot: a simple hunter. Edit this file -- improve targeting, add
// dodging, use the health pickups, whatever wins more matches.
//
// Run it with: node bot.js
// Play it locally with: arena tanks play . house:hunter house:sniper

const tanks = require('./tanks');

class Hunter {
  start(msg) {
    this.meId = msg.you;
  }

  decide(msg) {
    const me = msg.tanks.find((t) => t.id === this.meId);
    if (!me.alive) {
      return { move: 0, turn: 0, turret: 0, fire: false };
    }

    const target = tanks.nearestEnemy(me, msg.tanks);
    if (!target) {
      return { move: 0, turn: 0, turret: 0, fire: false };
    }

    // Turn the hull toward the target and drive in, unless we're already
    // close -- improve this: back off at close range, or pick a target by
    // lowest HP instead of nearest.
    const targetAngle = tanks.angleTo(me.x, me.y, target.x, target.y);
    const hullDiff = tanks.angleDiff(targetAngle, me.hull);
    const turn = Math.max(-1, Math.min(1, 3 * hullDiff));
    const dist = tanks.distance(me.x, me.y, target.x, target.y);
    const move = dist > 6 ? 1 : 0;

    // Aim the turret straight at the target -- improve this: lead moving
    // targets with tanks.leadTarget.
    const turretDiff = tanks.angleDiff(targetAngle, me.turret);
    const turret = Math.max(-1, Math.min(1, 4 * turretDiff));
    const fire = Math.abs(turretDiff) < 0.08;

    return { move, turn, turret, fire };
  }
}

tanks.run(new Hunter());
