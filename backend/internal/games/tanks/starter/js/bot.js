'use strict';
// Starter bot: a simple hunter. Edit this file -- improve targeting, add
// dodging, use the health pickups, whatever wins more matches.
//
// Run it with: node bot.js
// Play it locally with: arena tanks play . house:hunter house:sniper

const tanks = require('./tanks');

class Hunter {
  constructor() {
    this.stuck = new tanks.StuckWatcher();
  }

  start(msg) {
    this.meId = msg.you;
    this.startMsg = msg;
  }

  decide(msg) {
    const me = msg.tanks.find((t) => t.id === this.meId);
    if (!me.alive) {
      return { move: 0, turn: 0, turret: 0, fire: false };
    }

    const tick = msg.tick;

    // Wedged against a wall takes priority over everything else: back off
    // with a turn for a fixed number of ticks, then resume.
    if (this.stuck.backingOff() || this.stuck.stuck(tick, me.x, me.y)) {
      this.stuck.record(tick, me.x, me.y, -1);
      return { move: -1, turn: 1, turret: 0, fire: false };
    }

    const target = tanks.nearestEnemy(me, msg.tanks);
    if (!target) {
      this.stuck.record(tick, me.x, me.y, 0);
      return { move: 0, turn: 0, turret: 0, fire: false };
    }

    // Steer the hull toward the target, routing around any wall in
    // between instead of driving straight into it -- improve this: back
    // off at close range, or pick a target by lowest HP instead of
    // nearest.
    const lineClear = tanks.pathClear(this.startMsg, me, target);
    const heading = tanks.steerAround(this.startMsg, me, target);
    const hullDiff = tanks.angleDiff(heading, me.hull);
    const turn = Math.max(-1, Math.min(1, 3 * hullDiff));
    const dist = tanks.distance(me.x, me.y, target.x, target.y);
    // Keep closing until both close range AND a clear line of fire --
    // being within 6 units through a wall corner isn't "close enough".
    const move = dist > 6 || !lineClear ? 1 : 0;

    // Aim the turret straight at the target (not the steering heading),
    // and only fire when the line of fire is clear of walls -- no point
    // spending a reload on cover. Improve this: lead moving targets with
    // tanks.leadTarget.
    const targetAngle = tanks.angleTo(me.x, me.y, target.x, target.y);
    const turretDiff = tanks.angleDiff(targetAngle, me.turret);
    const turret = Math.max(-1, Math.min(1, 4 * turretDiff));
    const fire = Math.abs(turretDiff) < 0.08 && lineClear;

    this.stuck.record(tick, me.x, me.y, move);
    return { move, turn, turret, fire };
  }
}

tanks.run(new Hunter());
