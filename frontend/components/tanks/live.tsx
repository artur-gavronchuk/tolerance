'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { api, ApiError } from '@/lib/api'
import type { LiveView } from '@/lib/types'
import { fetchReplay, type Replay } from '@/lib/tanks/replay'
import { ReplayPlayer } from './viewer/replay-player'
import { Skeleton } from '@/components/ui/skeleton'

// Backoff for retrying after a fetch failure or an empty broadcast schedule
// (no ladder match has aired yet) - doubles each consecutive miss, capped at
// 30s, and reset to the floor the moment a real broadcast comes back.
const RETRY_FLOOR_MS = 5000
const RETRY_CAP_MS = 30000
// Never re-hit /tanks/live more often than this, regardless of what
// triggered the refetch (a match ending, catching up to a broadcast we
// loaded into mid-flight, or a retry) - the floor that keeps a fast client
// clock, or a short replay, from turning into a request-per-frame loop.
const MIN_REFETCH_MS = 3000

type Phase =
  | { kind: 'loading' }
  | { kind: 'empty' }
  | { kind: 'countdown'; seconds: number }
  // waiting: /tanks/live handed back a broadcast whose scheduled offset is
  // already at or past the end of the replay we have for it (clock skew, a
  // slow fetch, or a match that simply ran shorter than expected) - nothing
  // to play, so we sit tight until the broadcast's own end time instead of
  // looping straight back into another fetch.
  | { kind: 'waiting' }
  | { kind: 'playing'; matchId: string; replay: Replay; startTick: number; broadcastKey: string }
  | { kind: 'error'; message: string }

// The synchronized broadcast: everyone watching /tanks sees the same match
// at the same offset, computed from the server's own clock rather than the
// viewer's. The backend schedules a new broadcast 3 seconds in the future
// (`starts_at = now + 3s`) so every viewer's connector has time to load the
// replay before it needs to render anything — a viewer who loads mid
// pre-roll waits out a countdown and starts exactly at `starts_at`, rather
// than jumping in at tick 0 immediately and drifting out of sync with
// everyone else. When the current broadcast ends we ask /tanks/live again —
// it may hand back a new match, or repeat the same one, and either way the
// ReplayPlayer below is re-keyed by match id + broadcast start time (not by
// a counter that increments on every fetch), so a refetch that doesn't
// actually change what's airing never remounts the renderer - important in
// 3D, where a remount recreates the WebGL context.
export function Live() {
  const [phase, setPhase] = useState<Phase>({ kind: 'loading' })
  const matchIdRef = useRef<string | null>(null)
  const replayRef = useRef<Replay | null>(null)
  const startTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const tickTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const scheduleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastFetchAtRef = useRef(0)
  const retryDelayRef = useRef(RETRY_FLOOR_MS)
  // load() and scheduleLoad() are mutually recursive (a scheduled retry
  // calls back into load), so scheduleLoad reaches load through a ref
  // rather than a direct closure - keeps both as stable useCallbacks.
  const loadRef = useRef<() => void>(() => {})

  const clearTimers = useCallback(() => {
    if (startTimerRef.current) {
      clearTimeout(startTimerRef.current)
      startTimerRef.current = null
    }
    if (tickTimerRef.current) {
      clearInterval(tickTimerRef.current)
      tickTimerRef.current = null
    }
    if (scheduleTimerRef.current) {
      clearTimeout(scheduleTimerRef.current)
      scheduleTimerRef.current = null
    }
  }, [])

  // Schedules the next call into load(), at least delayMs from now.
  const scheduleLoad = useCallback((delayMs: number) => {
    if (scheduleTimerRef.current) clearTimeout(scheduleTimerRef.current)
    scheduleTimerRef.current = setTimeout(() => {
      scheduleTimerRef.current = null
      loadRef.current()
    }, Math.max(0, delayMs))
  }, [])

  // Requests a refetch no sooner than MIN_REFETCH_MS after the last one
  // actually started - used for the "normal" triggers (a match ending, or
  // catching up to a broadcast's end) that aren't already on a backoff.
  const requestLoad = useCallback(() => {
    scheduleLoad(MIN_REFETCH_MS - (Date.now() - lastFetchAtRef.current))
  }, [scheduleLoad])

  const load = useCallback(async () => {
    if (startTimerRef.current) {
      clearTimeout(startTimerRef.current)
      startTimerRef.current = null
    }
    if (tickTimerRef.current) {
      clearInterval(tickTimerRef.current)
      tickTimerRef.current = null
    }
    lastFetchAtRef.current = Date.now()
    try {
      const v = await api<LiveView>('/tanks/live')
      if (!v.match_id || !v.starts_at) {
        matchIdRef.current = null
        replayRef.current = null
        setPhase({ kind: 'empty' })
        const delay = retryDelayRef.current
        retryDelayRef.current = Math.min(RETRY_CAP_MS, retryDelayRef.current * 2)
        scheduleLoad(delay)
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
      const broadcastKey = `${matchId}-${v.starts_at}`

      // A real, well-formed broadcast came back - the backoff is only for
      // consecutive misses.
      retryDelayRef.current = RETRY_FLOOR_MS

      // Correct for the gap between the server's clock and this browser's:
      // skew is how far ahead the server is, so "now" on the server's
      // clock is Date.now() + skew.
      const skew = new Date(v.now).getTime() - Date.now()
      const startsAtMs = new Date(v.starts_at).getTime()
      const delayMs = startsAtMs - (Date.now() + skew)

      const begin = (offsetMs: number) => {
        setPhase({ kind: 'playing', matchId, replay, startTick: Math.max(0, offsetMs / 1000) * replay.tick_rate, broadcastKey })
      }

      if (delayMs > 0) {
        setPhase({ kind: 'countdown', seconds: Math.max(1, Math.ceil(delayMs / 1000)) })
        tickTimerRef.current = setInterval(() => {
          setPhase((p) => (p.kind === 'countdown' ? { kind: 'countdown', seconds: Math.max(0, p.seconds - 1) } : p))
        }, 1000)
        startTimerRef.current = setTimeout(() => {
          if (startTimerRef.current) {
            clearTimeout(startTimerRef.current)
            startTimerRef.current = null
          }
          if (tickTimerRef.current) {
            clearInterval(tickTimerRef.current)
            tickTimerRef.current = null
          }
          begin(0)
        }, delayMs)
        return
      }

      const offsetMs = -delayMs
      const maxTick = replay.frames.length ? replay.frames[replay.frames.length - 1].t : 0
      const offsetTick = (offsetMs / 1000) * replay.tick_rate
      if (offsetTick >= maxTick) {
        // Already past what we can actually play. Waiting here and playing
        // from tick 0 would show stale action out of sync with everyone
        // else, and refetching immediately is exactly the tight loop this
        // is guarding against - so sit out until the broadcast's own
        // scheduled end (skew-corrected), floored at MIN_REFETCH_MS.
        setPhase({ kind: 'waiting' })
        const endsAtLocal = startsAtMs - skew + v.duration_ms
        scheduleLoad(Math.max(MIN_REFETCH_MS, endsAtLocal - Date.now()))
        return
      }

      begin(offsetMs)
    } catch (e) {
      setPhase({ kind: 'error', message: (e as ApiError).message })
      const delay = retryDelayRef.current
      retryDelayRef.current = Math.min(RETRY_CAP_MS, retryDelayRef.current * 2)
      scheduleLoad(delay)
    }
  }, [scheduleLoad])

  useEffect(() => {
    loadRef.current = () => void load()
  }, [load])

  useEffect(() => {
    void load()
    return () => clearTimers()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

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

  if (phase.kind === 'waiting') {
    return (
      <div className="flex aspect-[3/2] w-full flex-col items-center justify-center gap-2 rounded-[18px] border border-dashed border-input bg-card text-center">
        <p className="font-mono text-2xl font-bold text-primary">Catching up…</p>
        <p className="text-sm text-muted-foreground">The next match will start shortly.</p>
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
      key={phase.broadcastKey}
      replay={phase.replay}
      startTick={phase.startTick}
      live
      autoPlay
      onEnd={requestLoad}
    />
  )
}
