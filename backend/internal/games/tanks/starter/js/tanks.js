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

module.exports = { log, angleTo, angleDiff, distance, nearestEnemy, leadTarget, run };
