import type { Replay, ReplayEvent } from './replay'

// Shared between the 2D canvas renderer and (task 15) the 3D one, so a
// bot's color never changes when the viewer does.
export const SLOT_COLORS = ['#e2573f', '#39a86b', '#3f7fe0', '#e0ab2e'] as const

export interface TankState { x: number; y: number; hull: number; turret: number; hp: number; alive: boolean }

export interface Snapshot {
  t: number
  tanks: TankState[]
  shells: { id: number; owner: number; x: number; y: number }[]
  bonuses: { x: number; y: number }[]
  zone: number
}

// Index of the last frame whose tick is <= t (frames are sorted by t).
function floorFrame(frames: Replay['frames'], t: number): number {
  let lo = 0
  let hi = frames.length - 1
  if (t <= frames[0].t) return 0
  if (t >= frames[hi].t) return hi
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1
    if (frames[mid].t <= t) lo = mid
    else hi = mid - 1
  }
  return lo
}

// Interpolates an angle along the shorter arc rather than wrapping the
// long way around when it crosses the ±π seam.
function lerpAngle(a: number, b: number, alpha: number): number {
  const twoPi = Math.PI * 2
  let diff = (((b - a) % twoPi) + twoPi + Math.PI) % twoPi
  diff -= Math.PI
  return a + diff * alpha
}

const lerp = (a: number, b: number, alpha: number) => a + (b - a) * alpha

// The interpolated game state at a fractional tick, for smooth playback
// between recorded frames (frames are one per tick; the clock runs
// continuously).
export function snapshotAt(r: Replay, tickFloat: number): Snapshot {
  const frames = r.frames
  if (frames.length === 0) return { t: 0, tanks: [], shells: [], bonuses: [], zone: 0 }

  const t = Math.min(Math.max(tickFloat, frames[0].t), frames[frames.length - 1].t)
  const i0 = floorFrame(frames, t)
  const i1 = Math.min(i0 + 1, frames.length - 1)
  const f0 = frames[i0]
  const f1 = frames[i1]
  const span = f1.t - f0.t
  const alpha = span > 0 ? (t - f0.t) / span : 0

  const slots = Math.max(f0.k.length, f1.k.length)
  const tanks: TankState[] = []
  for (let i = 0; i < slots; i++) {
    const a = f0.k[i]
    const b = f1.k[i] ?? a
    if (!a) continue
    tanks.push({
      x: lerp(a[0], b[0], alpha),
      y: lerp(a[1], b[1], alpha),
      hull: lerpAngle(a[2], b[2], alpha),
      turret: lerpAngle(a[3], b[3], alpha),
      hp: lerp(a[4], b[4], alpha),
      alive: (alpha < 1 ? a[6] : b[6]) === 1,
    })
  }

  const prevShells = new Map<number, number[]>()
  for (const s of f0.s) prevShells.set(s[0], s)
  const shells = f1.s.map((sb) => {
    const sa = prevShells.get(sb[0])
    return {
      id: sb[0],
      owner: sb[1],
      x: sa ? lerp(sa[2], sb[2], alpha) : sb[2],
      y: sa ? lerp(sa[3], sb[3], alpha) : sb[3],
    }
  })

  const bonuses = f1.b.map((b) => ({ x: b[0], y: b[1] }))
  const zone = lerp(f0.z, f1.z, alpha)

  return { t, tanks, shells, bonuses, zone }
}

// Events strictly after fromTick and up to and including toTick. Seeking
// backward (toTick <= fromTick) yields nothing — it's for "what just
// happened", not history.
export function eventsBetween(r: Replay, fromTick: number, toTick: number): ReplayEvent[] {
  if (toTick <= fromTick) return []
  return r.events.filter((e) => e.t > fromTick && e.t <= toTick)
}

const clampTick = (t: number, max: number) => Math.min(Math.max(t, 0), max)

// Renderer-agnostic playback clock: tracks a fractional tick position
// against wall-clock time so both the 2D canvas and (task 15) the 3D
// scene read the same `now()` every animation frame. Play/pause/seek/speed
// only touch a small amount of state; the actual tick position is
// computed lazily in now() from performance.now(), so nothing needs to
// poll it to stay correct.
export class Clock {
  private tickPos = 0
  private playing = false
  private playStartWall = 0
  private playStartTick = 0
  private speed = 1
  private readonly maxTick: number
  private ended = false
  onEnd?: () => void

  constructor(private readonly replay: Replay) {
    this.maxTick = replay.frames.length ? replay.frames[replay.frames.length - 1].t : 0
  }

  play(): void {
    if (this.playing) return
    this.playing = true
    this.ended = false
    this.playStartWall = performance.now()
    this.playStartTick = this.tickPos
  }

  pause(): void {
    if (!this.playing) return
    this.tickPos = this.now()
    this.playing = false
  }

  seek(tick: number): void {
    const clamped = clampTick(tick, this.maxTick)
    this.tickPos = clamped
    this.ended = false
    if (this.playing) {
      this.playStartWall = performance.now()
      this.playStartTick = clamped
    }
  }

  setSpeed(x: number): void {
    if (this.playing) {
      this.tickPos = this.now()
      this.playStartWall = performance.now()
      this.playStartTick = this.tickPos
    }
    this.speed = x
  }

  now(): number {
    if (!this.playing) return this.tickPos
    const elapsedS = (performance.now() - this.playStartWall) / 1000
    const t = this.playStartTick + elapsedS * this.replay.tick_rate * this.speed
    if (t >= this.maxTick) {
      if (!this.ended) {
        this.ended = true
        this.playing = false
        this.tickPos = this.maxTick
        // Deferred: firing mid-read (during a render's now() call) could
        // trigger a state update in the same tick as this read.
        queueMicrotask(() => this.onEnd?.())
      }
      return this.maxTick
    }
    return t
  }

  isPlaying(): boolean {
    return this.playing
  }

  duration(): number {
    return this.maxTick
  }
}
