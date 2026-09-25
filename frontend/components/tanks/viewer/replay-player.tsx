'use client'

import { useEffect, useMemo, useRef, useState } from 'react'
import { Clock, snapshotAt } from '@/lib/tanks/playback'
import type { Replay, ReplayEvent } from '@/lib/tanks/replay'
import { Canvas2D } from './canvas2d'
import { Controls } from './controls'
import { Scoreboard } from './scoreboard'
import { EventFeed } from './event-feed'

// Every notable event (everything but "shot") up to and including `t`,
// newest first — recomputed from scratch on every tick change so scrubbing
// backward and forward both just work.
function recentEvents(replay: Replay, t: number, count: number): ReplayEvent[] {
  const out: ReplayEvent[] = []
  for (let i = replay.events.length - 1; i >= 0 && out.length < count; i--) {
    const e = replay.events[i]
    if (e.t > t) continue
    if (e.e === 'shot') continue
    out.push(e)
  }
  return out
}

function killsBySlotAt(replay: Replay, t: number): number[] {
  const counts = new Array(replay.players.length).fill(0)
  for (const e of replay.events) {
    if (e.t > t) break
    if (e.e === 'kill' && e.a >= 0 && e.a < counts.length) counts[e.a]++
  }
  return counts
}

// Owns the Clock and orchestrates the renderer plus the surrounding UI
// (controls, scoreboard, event feed). The renderer switch between 2D and
// 3D (task 15) plugs into the same Clock, so switching never resets
// playback.
export function ReplayPlayer({ replay, startTick = 0, live = false, autoPlay = true, onEnd }: {
  replay: Replay
  startTick?: number
  live?: boolean
  autoPlay?: boolean
  onEnd?: () => void
}) {
  const clock = useMemo(() => {
    const c = new Clock(replay)
    c.seek(startTick)
    if (autoPlay) c.play()
    return c
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [replay])

  useEffect(() => {
    clock.onEnd = onEnd
  }, [clock, onEnd])

  const [playing, setPlaying] = useState(clock.isPlaying())
  const [speed, setSpeed] = useState(1)
  const [tick, setTick] = useState(startTick)
  const lastUiRef = useRef(0)

  useEffect(() => {
    let raf = 0
    function loop(nowMs: number) {
      if (nowMs - lastUiRef.current > 90) {
        lastUiRef.current = nowMs
        setTick(clock.now())
        setPlaying(clock.isPlaying())
      }
      raf = requestAnimationFrame(loop)
    }
    raf = requestAnimationFrame(loop)
    return () => cancelAnimationFrame(raf)
  }, [clock])

  const feed = useMemo(() => recentEvents(replay, tick, 8), [replay, tick])
  const killsBySlot = useMemo(() => killsBySlotAt(replay, tick), [replay, tick])
  // Only needed for the scoreboard's HP bars, at UI refresh rate — the
  // canvas computes its own snapshots independently on its own 60fps loop.
  const tanks = useMemo(() => snapshotAt(replay, tick).tanks, [replay, tick])

  function handlePlayPause() {
    if (clock.isPlaying()) clock.pause()
    else clock.play()
    setPlaying(clock.isPlaying())
    setTick(clock.now())
  }
  function handleSpeed(x: number) {
    clock.setSpeed(x)
    setSpeed(x)
  }
  function handleSeek(t: number) {
    clock.seek(t)
    setTick(clock.now())
  }

  return (
    <div className="flex flex-col gap-4 lg:flex-row lg:items-start">
      <div className="min-w-0 flex-1 space-y-3">
        <Canvas2D replay={replay} clock={clock} />
        <Controls
          playing={playing}
          onPlayPause={handlePlayPause}
          speed={speed}
          onSpeed={handleSpeed}
          tick={tick}
          maxTick={clock.duration()}
          onSeek={handleSeek}
          live={live}
          tickRate={replay.tick_rate}
        />
      </div>
      <div className="flex w-full flex-col gap-4 lg:w-72 lg:shrink-0">
        <Scoreboard players={replay.players} tanks={tanks} killsBySlot={killsBySlot} />
        <EventFeed events={feed} players={replay.players} />
      </div>
    </div>
  )
}
