'use strict';
// tanks.js -- protocol and helper library for tanks starter bots.
//
// Import this from your bot and call run(strategy). See bot.js for an
// example strategy and GAME.md for the rules and protocol this wraps.
//
// No third-party dependencies -- standard library only, as required by the
// package rules.

const readline = require('readline');

function log(...args) {
  console.error(...args);
}

function angleTo(x, y, tx, ty) {
  return Math.atan2(ty - y, tx - x);
}

function angleDiff(a, b) {
  let d = ((a - b + Math.PI) % (2 * Math.PI)) - Math.PI;
  if (d <= -Math.PI) d += 2 * Math.PI;
  return d;
}

function distance(x, y, tx, ty) {
  return Math.hypot(tx - x, ty - y);
}

function nearestEnemy(me, tanksList) {
  let best = null;
  let bestD = null;
  for (const t of tanksList) {
    if (t.id === me.id || !t.alive) continue;
    const d = distance(me.x, me.y, t.x, t.y);
    if (bestD === null || d < bestD) {
      best = t;
      bestD = d;
    }
  }
  return best;
}

function leadTarget(shooter, target, shellSpeed, targetVx = 0, targetVy = 0) {
  const dx = target.x - shooter.x;
  const dy = target.y - shooter.y;

  const a = targetVx * targetVx + targetVy * targetVy - shellSpeed * shellSpeed;
  const b = 2 * (dx * targetVx + dy * targetVy);
  const c = dx * dx + dy * dy;

  let t = null;
  if (Math.abs(a) < 1e-9) {
    if (Math.abs(b) > 1e-9) {
      const candidate = -c / b;
      if (candidate > 0) t = candidate;
    }
  } else {
    const disc = b * b - 4 * a * c;
    if (disc >= 0) {
      const sq = Math.sqrt(disc);
      const candidates = [(-b + sq) / (2 * a), (-b - sq) / (2 * a)].filter((x) => x > 0);
      if (candidates.length > 0) t = Math.min(...candidates);
    }
  }

  if (t === null) return { x: target.x, y: target.y };
  return { x: target.x + targetVx * t, y: target.y + targetVy * t };
}

// Wall avoidance. bunkers's per-spawn L-shaped walls trap a bot that only
// ever turns straight toward its target -- pathClear and steerAround give
// you a cheap way to check a line against the map and route around an
// obstacle instead of driving into it.

const STEER_MARGIN = 0.4; // extra clearance beyond the tank's own radius
const STEER_LOOKAHEAD = 6.0; // how far ahead a candidate heading is checked, in units
const STEER_OFFSETS_DEG = [30, 60, 90, 120]; // tried in order, nearest first

// segmentClearsRect reports whether the segment (x1,y1)-(x2,y2) stays clear
// of rect (an {x,y,w,h} object) inflated by pad on every side.
// Liang-Barsky segment-vs-AABB clipping: walk the segment's parametric
// range [0, 1] through each of the rect's four half-plane constraints -- it
// only enters the rect if a non-empty sub-range survives all four.
function segmentClearsRect(x1, y1, x2, y2, rect, pad) {
  const rx0 = rect.x - pad;
  const ry0 = rect.y - pad;
  const rx1 = rect.x + rect.w + pad;
  const ry1 = rect.y + rect.h + pad;
  const dx = x2 - x1;
  const dy = y2 - y1;
  let t0 = 0;
  let t1 = 1;
  const edges = [
    [-dx, x1 - rx0],
    [dx, rx1 - x1],
    [-dy, y1 - ry0],
    [dy, ry1 - y1],
  ];
  for (const [p, q] of edges) {
    if (Math.abs(p) < 1e-9) {
      if (q < 0) return true; // parallel to this edge and outside it: never enters
      continue;
    }
    const r = q / p;
    if (p < 0) {
      if (r > t1) return true;
      t0 = Math.max(t0, r);
    } else {
      if (r < t0) return true;
      t1 = Math.min(t1, r);
    }
  }
  return t0 > t1; // empty surviving interval: the segment never enters the rect
}

// pathClear reports whether a straight line from `from` to `to` (each an
// {x, y} object) stays clear of every wall in `start` (the start message),
// each inflated by the tank's own radius plus a small margin. Use this
// before committing to a heading, and before firing, so you don't drive --
// or shoot -- straight into cover.
function pathClear(start, from, to) {
  const pad = start.rules.tank_radius + STEER_MARGIN;
  return start.walls.every((wall) => segmentClearsRect(from.x, from.y, to.x, to.y, wall, pad));
}

// steerAround returns the heading (radians) to drive from me toward target
// (each an {x, y} object), routing around walls: the direct heading if
// that line is clear, otherwise the first clear heading among the
// +-30/60/90/120 degree offsets in STEER_OFFSETS_DEG, checked
// STEER_LOOKAHEAD units ahead and preferring whichever side of a given
// offset closes more distance to target. Falls back to the direct heading
// if every offset is blocked, so a boxed-in tank still pushes toward its
// target rather than freezing.
function steerAround(start, me, target) {
  const direct = angleTo(me.x, me.y, target.x, target.y);
  if (pathClear(start, me, target)) return direct;

  for (const deg of STEER_OFFSETS_DEG) {
    let best = direct;
    let bestGain = null;
    for (const sign of [1, -1]) {
      const heading = direct + (sign * deg * Math.PI) / 180;
      const look = { x: me.x + STEER_LOOKAHEAD * Math.cos(heading), y: me.y + STEER_LOOKAHEAD * Math.sin(heading) };
      if (!pathClear(start, me, look)) continue;
      const gain = distance(me.x, me.y, target.x, target.y) - distance(look.x, look.y, target.x, target.y);
      if (bestGain === null || gain > bestGain) {
        best = heading;
        bestGain = gain;
      }
    }
    if (bestGain !== null) return best;
  }
  return direct;
}

// StuckWatcher detects a tank that was told to move but barely moved over
// the last `window` ticks -- wedged against a wall -- and then walks the
// caller through a fixed reverse-and-turn recovery for `backoffTicks`
// ticks.
//
// Call backingOff() and stuck() in that order at the top of decide(); if
// either returns true, issue { move: -1, turn: 1, ... } for this tick
// instead of your normal plan. Either way, call record() once per tick
// with the position you're at and the move you issued, so the next tick's
// check has this tick's sample to compare against.
class StuckWatcher {
  constructor(window = 8, moveThreshold = 0.2, backoffTicks = 6) {
    this.window = window;
    this.moveThreshold = moveThreshold;
    this.backoffTicks = backoffTicks;
    this.history = []; // {tick, x, y}, oldest first, trimmed to `window` samples
    this.moves = []; // move issued each of the last `window` ticks
    this.backoffLeft = 0;
  }

  backingOff() {
    if (this.backoffLeft <= 0) return false;
    this.backoffLeft -= 1;
    return true;
  }

  // history holds exactly `window` samples once warmed up (the ticks
  // immediately before this one), so its oldest entry is `window` ticks
  // behind the current tick -- the comparison this method needs.
  stuck(tick, x, y) {
    if (this.history.length < this.window) return false;
    const oldest = this.history[0];
    if (tick - oldest.tick !== this.window) return false;
    if (distance(oldest.x, oldest.y, x, y) > this.moveThreshold) return false;
    if (!this.moves.some((m) => m !== 0)) return false;
    this.backoffLeft = this.backoffTicks - 1;
    return true;
  }

  record(tick, x, y, move) {
    this.history.push({ tick, x, y });
    this.moves.push(move);
    if (this.history.length > this.window) this.history = this.history.slice(-this.window);
    if (this.moves.length > this.window) this.moves = this.moves.slice(-this.window);
  }
}

function run(strategy) {
  const rl = readline.createInterface({ input: process.stdin, terminal: false });

  rl.on('line', (raw) => {
    const line = raw.trim();
    if (!line) return;
    let msg;
    try {
      msg = JSON.parse(line);
    } catch (e) {
      return;
    }
    if (typeof msg !== 'object' || msg === null || Array.isArray(msg)) return;

    switch (msg.type) {
      case 'start':
        if (typeof strategy.start === 'function') strategy.start(msg);
        process.stdout.write(JSON.stringify({ type: 'ready' }) + '\n');
        break;
      case 'tick': {
        const reply = strategy.decide(msg);
        reply.tick = msg.tick;
        process.stdout.write(JSON.stringify(reply) + '\n');
        break;
      }
      case 'end':
        rl.close();
        process.exit(0);
        break;
      default:
        // Unknown message types are ignored, not an error -- future
        // protocol additions should never crash an existing bot.
        break;
    }
  });

  rl.on('close', () => {
    process.exit(0);
  });
}

module.exports = {
  log,
  angleTo,
  angleDiff,
  distance,
  nearestEnemy,
  leadTarget,
  pathClear,
  steerAround,
  StuckWatcher,
  run,
};
