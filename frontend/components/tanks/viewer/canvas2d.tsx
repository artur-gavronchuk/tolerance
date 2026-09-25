'use client'

import { useEffect, useRef } from 'react'
import { eventsBetween, snapshotAt, SLOT_COLORS, type Snapshot } from '@/lib/tanks/playback'
import type { Clock } from '@/lib/tanks/playback'
import type { Replay } from '@/lib/tanks/replay'

// Effect lifetimes are in simulation ticks, not wall-clock time: an effect
// is tied to the replay's own clock, so it freezes exactly like everything
// else while paused, and fades over a consistent number of ticks
// regardless of playback speed rather than a fixed number of real seconds.
interface ShotFx { x: number; y: number; bornTick: number }
interface HitFx { slot: number; bornTick: number }
interface KillFx { x: number; y: number; bornTick: number }

const SHOT_LIFE_TICKS = 1.5
const HIT_LIFE_TICKS = 2.6
const KILL_LIFE_TICKS = 5.2

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  ctx.beginPath()
  ctx.moveTo(x + r, y)
  ctx.arcTo(x + w, y, x + w, y + h, r)
  ctx.arcTo(x + w, y + h, x, y + h, r)
  ctx.arcTo(x, y + h, x, y, r)
  ctx.arcTo(x, y, x + w, y, r)
  ctx.closePath()
}

// The 2D battlefield renderer. Takes the same { replay, clock } contract
// the (task 15) 3D renderer will use, so the viewer can switch between
// them without resetting playback. Owns its own requestAnimationFrame
// loop while the clock is playing — drawing at 60fps is cheap on a canvas
// but would be wasteful to drive through React state, so nothing here is
// React state except the canvas element itself. The loop stops entirely
// while paused (nothing on screen can change), and a single frame is
// redrawn on resize and on every play/pause/seek/speed change instead,
// via Clock.subscribe.
export function Canvas2D({ replay, clock }: { replay: Replay; clock: Clock }) {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const canvasRef = useRef<HTMLCanvasElement | null>(null)

  useEffect(() => {
    const container = containerRef.current
    const canvas = canvasRef.current
    if (!container || !canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return

    let cssW = 0
    let cssH = 0
    let raf = 0

    function resize() {
      const rect = container!.getBoundingClientRect()
      const dpr = Math.min(window.devicePixelRatio || 1, 3)
      cssW = Math.max(1, Math.round(rect.width))
      cssH = Math.max(1, Math.round(rect.height))
      canvas!.width = Math.round(cssW * dpr)
      canvas!.height = Math.round(cssH * dpr)
      ctx!.setTransform(dpr, 0, 0, dpr, 0, 0)
      render()
    }
    const ro = new ResizeObserver(resize)
    ro.observe(container)

    let lastTick = clock.now()
    const shots: ShotFx[] = []
    const hits: HitFx[] = []
    const kills: KillFx[] = []

    function fieldToPx(x: number, y: number, scale: number): [number, number] {
      return [x * scale, cssH - y * scale]
    }

    function collectEffects(t: number) {
      if (t < lastTick) {
        // A rewind — any pending flash is no longer meaningful relative to
        // the new playhead; events between here and lastTick will
        // repopulate it as we play forward again.
        shots.length = 0
        hits.length = 0
        kills.length = 0
      }
      const evs = eventsBetween(replay, lastTick, t)
      lastTick = t
      const rules = replay.rules
      for (const e of evs) {
        if (e.e === 'shot') {
          const s = snapshotAt(replay, e.t)
          const tank = s.tanks[e.a]
          if (tank) {
            shots.push({
              x: tank.x + Math.cos(tank.turret) * rules.muzzle_offset,
              y: tank.y + Math.sin(tank.turret) * rules.muzzle_offset,
              bornTick: e.t,
            })
          }
        } else if (e.e === 'hit' && e.b != null) {
          hits.push({ slot: e.b, bornTick: e.t })
        } else if (e.e === 'kill' && e.b != null) {
          const s = snapshotAt(replay, e.t)
          const tank = s.tanks[e.b]
          if (tank) kills.push({ x: tank.x, y: tank.y, bornTick: e.t })
        }
      }
    }

    function drawTank(x: number, y: number, hull: number, turret: number, hp: number, alive: boolean, color: string, scale: number) {
      const [px, py] = fieldToPx(x, y, scale)
      const bodyLen = scale * 1.7
      const bodyW = scale * 1.2

      ctx!.save()
      ctx!.translate(px, py)
      ctx!.rotate(-hull)
      ctx!.globalAlpha = alive ? 1 : 0.4
      roundRect(ctx!, -bodyLen / 2, -bodyW / 2, bodyLen, bodyW, scale * 0.28)
      ctx!.fillStyle = alive ? color : '#5b6b78'
      ctx!.fill()
      ctx!.strokeStyle = 'rgba(0,0,0,0.35)'
      ctx!.lineWidth = 1
      ctx!.stroke()
      ctx!.restore()

      if (!alive) return

      ctx!.save()
      ctx!.translate(px, py)
      ctx!.rotate(-turret)
      ctx!.fillStyle = 'rgba(0,0,0,0.3)'
      ctx!.fillRect(0, -scale * 0.09, scale * 1.35, scale * 0.18)
      ctx!.restore()

      ctx!.save()
      ctx!.translate(px, py)
      ctx!.beginPath()
      ctx!.arc(0, 0, scale * 0.55, 0, Math.PI * 2)
      ctx!.fillStyle = color
      ctx!.fill()
      ctx!.strokeStyle = 'rgba(0,0,0,0.35)'
      ctx!.stroke()
      ctx!.restore()

      const barW = scale * 1.5
      const by = py - scale * 1.2
      const frac = Math.max(0, Math.min(1, hp / 100))
      ctx!.fillStyle = 'rgba(0,0,0,0.45)'
      ctx!.fillRect(px - barW / 2, by, barW, 4)
      ctx!.fillStyle = frac > 0.4 ? '#52ba8a' : frac > 0.15 ? '#e1a64a' : '#ee6b62'
      ctx!.fillRect(px - barW / 2, by, barW * frac, 4)
    }

    function render() {
      if (cssW === 0) return
      const t = clock.now()
      collectEffects(t)
      const snap: Snapshot = snapshotAt(replay, t)
      const rules = replay.rules
      const scale = cssW / rules.width

      ctx!.clearRect(0, 0, cssW, cssH)
      ctx!.fillStyle = '#0c1720'
      ctx!.fillRect(0, 0, cssW, cssH)

      // Walls
      ctx!.fillStyle = '#26333d'
      for (const w of replay.walls) {
        const [wx, wy] = fieldToPx(w.x, w.y + w.h, scale)
        ctx!.fillRect(wx, wy, w.w * scale, w.h * scale)
      }

      // Shrinking zone: dashed ring + dim outside, only once it has
      // started shrinking (it sits at the full field radius before that).
      if (snap.zone < rules.zone_start_radius - 0.01) {
        const [zx, zy] = fieldToPx(rules.width / 2, rules.height / 2, scale)
        const zr = snap.zone * scale
        ctx!.save()
        ctx!.beginPath()
        ctx!.rect(0, 0, cssW, cssH)
        ctx!.moveTo(zx + zr, zy)
        ctx!.arc(zx, zy, zr, 0, Math.PI * 2)
        ctx!.closePath()
        ctx!.fillStyle = 'rgba(4,8,11,0.55)'
        ctx!.fill('evenodd')
        ctx!.setLineDash([6, 5])
        ctx!.strokeStyle = 'rgba(238,107,98,0.85)'
        ctx!.lineWidth = 1.5
        ctx!.beginPath()
        ctx!.arc(zx, zy, zr, 0, Math.PI * 2)
        ctx!.stroke()
        ctx!.restore()
      }

      // Bonuses (repair kits)
      for (const b of snap.bonuses) {
        const [bx, by] = fieldToPx(b.x, b.y, scale)
        const r = Math.max(4, scale * 0.5)
        ctx!.save()
        ctx!.translate(bx, by)
        ctx!.beginPath()
        ctx!.arc(0, 0, r, 0, Math.PI * 2)
        ctx!.fillStyle = '#2e7d5b'
        ctx!.fill()
        ctx!.strokeStyle = '#eef2f5'
        ctx!.lineWidth = Math.max(1.5, scale * 0.06)
        ctx!.beginPath()
        ctx!.moveTo(-r * 0.45, 0)
        ctx!.lineTo(r * 0.45, 0)
        ctx!.moveTo(0, -r * 0.45)
        ctx!.lineTo(0, r * 0.45)
        ctx!.stroke()
        ctx!.restore()
      }

      // Shells
      for (const s of snap.shells) {
        const [sx, sy] = fieldToPx(s.x, s.y, scale)
        ctx!.beginPath()
        ctx!.fillStyle = '#ffd873'
        ctx!.arc(sx, sy, Math.max(2, scale * 0.12), 0, Math.PI * 2)
        ctx!.fill()
      }

      // Tanks
      snap.tanks.forEach((tank, i) => {
        const color = SLOT_COLORS[i % SLOT_COLORS.length]
        drawTank(tank.x, tank.y, tank.hull, tank.turret, tank.hp, tank.alive, color, scale)
      })

      // Effects: muzzle flash, hit blink, kill burst — aged in simulation
      // ticks against the current tick, so they hold still while paused.
      for (let i = shots.length - 1; i >= 0; i--) {
        const fx = shots[i]
        const age = t - fx.bornTick
        if (age > SHOT_LIFE_TICKS || age < 0) { shots.splice(i, 1); continue }
        const [px, py] = fieldToPx(fx.x, fx.y, scale)
        const frac = 1 - age / SHOT_LIFE_TICKS
        ctx!.save()
        ctx!.globalAlpha = Math.max(0, Math.min(1, frac))
        ctx!.fillStyle = '#ffe38a'
        ctx!.beginPath()
        ctx!.arc(px, py, scale * 0.32 * (0.6 + frac), 0, Math.PI * 2)
        ctx!.fill()
        ctx!.restore()
      }
      for (let i = hits.length - 1; i >= 0; i--) {
        const fx = hits[i]
        const age = t - fx.bornTick
        if (age > HIT_LIFE_TICKS || age < 0) { hits.splice(i, 1); continue }
        const tank = snap.tanks[fx.slot]
        if (!tank) continue
        const [px, py] = fieldToPx(tank.x, tank.y, scale)
        const frac = 1 - age / HIT_LIFE_TICKS
        ctx!.save()
        ctx!.globalAlpha = Math.max(0, Math.min(1, frac))
        ctx!.strokeStyle = '#ee6b62'
        ctx!.lineWidth = 2
        ctx!.beginPath()
        ctx!.arc(px, py, scale * (0.75 + (1 - frac) * 0.5), 0, Math.PI * 2)
        ctx!.stroke()
        ctx!.restore()
      }
      for (let i = kills.length - 1; i >= 0; i--) {
        const fx = kills[i]
        const age = t - fx.bornTick
        if (age > KILL_LIFE_TICKS || age < 0) { kills.splice(i, 1); continue }
        const [px, py] = fieldToPx(fx.x, fx.y, scale)
        const frac = age / KILL_LIFE_TICKS
        ctx!.save()
        ctx!.globalAlpha = Math.max(0, 1 - frac)
        ctx!.strokeStyle = '#ffb454'
        ctx!.lineWidth = 2.5
        ctx!.beginPath()
        ctx!.arc(px, py, scale * (0.4 + frac * 1.9), 0, Math.PI * 2)
        ctx!.stroke()
        ctx!.restore()
      }
    }

    function loop() {
      render()
      raf = requestAnimationFrame(loop)
    }
    function start() {
      if (raf) return
      raf = requestAnimationFrame(loop)
    }
    function stop() {
      if (raf) cancelAnimationFrame(raf)
      raf = 0
    }

    const unsubscribe = clock.subscribe(() => {
      if (clock.isPlaying()) {
        start()
      } else {
        stop()
        render()
      }
    })

    resize() // sizes the canvas and renders the first frame
    if (clock.isPlaying()) start()

    return () => {
      stop()
      ro.disconnect()
      unsubscribe()
    }
  }, [replay, clock])

  return (
    <div ref={containerRef} className="relative w-full overflow-hidden rounded-[14px] bg-[#0c1720]" style={{ aspectRatio: '3 / 2' }}>
      <canvas ref={canvasRef} className="block h-full w-full" />
    </div>
  )
}
