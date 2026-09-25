'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { api, ApiError } from '@/lib/api'
import type { LiveView } from '@/lib/types'
import { fetchReplay, type Replay } from '@/lib/tanks/replay'
import { ReplayPlayer } from './viewer/replay-player'
import { Skeleton } from '@/components/ui/skeleton'

type Phase =
  | { kind: 'loading' }
  | { kind: 'empty' }
  | { kind: 'countdown'; seconds: number }
  | { kind: 'playing'; matchId: string; replay: Replay; startTick: number; epoch: number }
  | { kind: 'error'; message: string }

// The synchronized broadcast: everyone watching /tanks sees the same match
// at the same offset, computed from the server's own clock rather than the
// viewer's. The backend schedules a new broadcast 3 seconds in the future
// (`starts_at = now + 3s`) so every viewer's connector has time to load the
// replay before it needs to render anything — a viewer who loads mid
// pre-roll waits out a countdown and starts exactly at `starts_at`, rather
// than jumping in at tick 0 immediately and drifting out of sync with
// everyone else. When the current broadcast ends we just ask /tanks/live
// again — it may hand back a new match, or repeat the same one, and either
// way `epoch` forces the player to restart clean.
export function Live() {
  const [phase, setPhase] = useState<Phase>({ kind: 'loading' })
  const matchIdRef = useRef<string | null>(null)
  const replayRef = useRef<Replay | null>(null)
  const epochRef = useRef(0)
  const startTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const tickTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const clearTimers = useCallback(() => {
    if (startTimerRef.current) {
      clearTimeout(startTimerRef.current)
      startTimerRef.current = null
    }
    if (tickTimerRef.current) {
      clearInterval(tickTimerRef.current)
      tickTimerRef.current = null
    }
  }, [])

  const load = useCallback(async () => {
    clearTimers()
    try {
      const v = await api<LiveView>('/tanks/live')
      if (!v.match_id || !v.starts_at) {
        matchIdRef.current = null
        replayRef.current = null
        setPhase({ kind: 'empty' })
        return
      }
      let r = replayRef.current
      if (v.match_id !== matchIdRef.current) {
        r = await fetchReplay(v.match_id)
        replayRef.current = r
        matchIdRef.current = v.match_id
      }
      if (!r) return
      const matchId = v.match_id
      const replay = r

      // Correct for the gap between the server's clock and this browser's:
      // skew is how far ahead the server is, so "now" on the server's
      // clock is Date.now() + skew.
      const skew = new Date(v.now).getTime() - Date.now()
      const startsAtMs = new Date(v.starts_at).getTime()
      const delayMs = startsAtMs - (Date.now() + skew)

      const begin = (offsetMs: number) => {
        epochRef.current += 1
        setPhase({ kind: 'playing', matchId, replay, startTick: Math.max(0, offsetMs / 1000) * replay.tick_rate, epoch: epochRef.current })
      }

      if (delayMs > 0) {
        setPhase({ kind: 'countdown', seconds: Math.max(1, Math.ceil(delayMs / 1000)) })
        tickTimerRef.current = setInterval(() => {
          setPhase((p) => (p.kind === 'countdown' ? { kind: 'countdown', seconds: Math.max(0, p.seconds - 1) } : p))
        }, 1000)
        startTimerRef.current = setTimeout(() => {
          clearTimers()
          begin(0)
        }, delayMs)
      } else {
        begin(-delayMs)
      }
    } catch (e) {
      setPhase({ kind: 'error', message: (e as ApiError).message })
    }
  }, [clearTimers])

  useEffect(() => {
    void load()
    return () => clearTimers()
  }, [load, clearTimers])

  if (phase.kind === 'loading') return <Skeleton className="aspect-[3/2] w-full rounded-[18px]" />

  if (phase.kind === 'error') {
    return (
      <p className="rounded-[18px] border border-dashed border-input px-6 py-14 text-center text-sm text-muted-foreground">
        Couldn&apos;t reach the live broadcast: {phase.message}
      </p>
    )
  }

  if (phase.kind === 'empty') {
    return (
      <div className="rounded-[18px] border border-dashed border-input px-6 py-14 text-center">
        <p className="text-sm font-semibold text-muted-foreground">No matches yet.</p>
        <p className="mt-2 text-sm text-muted-foreground">
          The ladder starts running once a bot is registered.{' '}
          <Link href="/tanks/docs" className="font-semibold text-primary hover:underline">
            Read the docs
          </Link>{' '}
          to get one in.
        </p>
      </div>
    )
  }

  if (phase.kind === 'countdown') {
    return (
      <div className="flex aspect-[3/2] w-full flex-col items-center justify-center gap-2 rounded-[18px] border border-dashed border-input bg-card text-center">
        <p className="font-mono text-2xl font-bold text-primary">Starting in {phase.seconds}…</p>
        <p className="text-sm text-muted-foreground">The next match is about to go live.</p>
      </div>
    )
  }

  return (
    <ReplayPlayer
      key={`${phase.matchId}-${phase.epoch}`}
      replay={phase.replay}
      startTick={phase.startTick}
      live
      autoPlay
      onEnd={() => void load()}
    />
  )
}
