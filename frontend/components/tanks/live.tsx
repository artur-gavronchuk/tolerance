'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { api, ApiError } from '@/lib/api'
import type { LiveView } from '@/lib/types'
import { fetchReplay, type Replay } from '@/lib/tanks/replay'
import { ReplayPlayer } from './viewer/replay-player'
import { Skeleton } from '@/components/ui/skeleton'

// The synchronized broadcast: everyone watching /tanks sees the same
// match at the same offset, computed from the server's own clock rather
// than the viewer's. When the current broadcast ends we just ask
// /tanks/live again — it may hand back a new match, or repeat the same
// one, and either way `epoch` forces the player to restart clean.
export function Live() {
  const [live, setLive] = useState<LiveView | null>(null)
  const [replay, setReplay] = useState<Replay | null>(null)
  const [startTick, setStartTick] = useState(0)
  const [epoch, setEpoch] = useState(0)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const matchIdRef = useRef<string | null>(null)
  const replayRef = useRef<Replay | null>(null)

  const load = useCallback(async () => {
    try {
      const v = await api<LiveView>('/tanks/live')
      setLive(v)
      setError(null)
      if (!v.match_id || !v.starts_at) {
        setReplay(null)
        matchIdRef.current = null
        replayRef.current = null
        setLoading(false)
        return
      }
      let r = replayRef.current
      if (v.match_id !== matchIdRef.current) {
        r = await fetchReplay(v.match_id)
        replayRef.current = r
        matchIdRef.current = v.match_id
      }
      if (!r) return
      // Correct for the gap between the server's clock and this browser's:
      // skew is how far ahead the server is, so "now" on the server's
      // clock is Date.now() + skew.
      const skew = new Date(v.now).getTime() - Date.now()
      const offsetMs = Date.now() + skew - new Date(v.starts_at).getTime()
      setReplay(r)
      setStartTick(Math.max(0, (offsetMs / 1000) * r.tick_rate))
      setEpoch((n) => n + 1)
    } catch (e) {
      setError((e as ApiError).message)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  if (loading) return <Skeleton className="aspect-[3/2] w-full rounded-[18px]" />

  if (error) {
    return (
      <p className="rounded-[18px] border border-dashed border-input px-6 py-14 text-center text-sm text-muted-foreground">
        Couldn&apos;t reach the live broadcast: {error}
      </p>
    )
  }

  if (!live?.match_id || !replay) {
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

  return <ReplayPlayer key={`${live.match_id}-${epoch}`} replay={replay} startTick={startTick} live autoPlay onEnd={() => void load()} />
}
