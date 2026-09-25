'use client'

import { use, useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Check, Copy } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { ReplayPlayer } from '@/components/tanks/viewer/replay-player'
import { BotBadge } from '@/components/tanks/bot-badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { api, ApiError } from '@/lib/api'
import { fetchReplay, type Replay } from '@/lib/tanks/replay'
import type { MatchView } from '@/lib/types'

function ratingDelta(before: number | null, after: number | null): string {
  if (before == null || after == null) return '—'
  const d = after - before
  return d === 0 ? '±0' : d > 0 ? `+${d}` : `${d}`
}

type ReplayState =
  | { kind: 'idle' }
  | { kind: 'loading' }
  | { kind: 'ready'; replay: Replay }
  | { kind: 'expired' }
  | { kind: 'unsupported' }
  | { kind: 'error'; message: string }

export default function MatchPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [match, setMatch] = useState<MatchView | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [replayState, setReplayState] = useState<ReplayState>({ kind: 'idle' })
  const [copied, setCopied] = useState(false)
  const aliveRef = useRef(true)

  const loadReplay = useCallback(async (matchId: string) => {
    if (typeof DecompressionStream === 'undefined') {
      setReplayState({ kind: 'unsupported' })
      return
    }
    setReplayState({ kind: 'loading' })
    try {
      const r = await fetchReplay(matchId)
      if (aliveRef.current) setReplayState({ kind: 'ready', replay: r })
    } catch (e) {
      if (!aliveRef.current) return
      if (e instanceof ApiError && e.status === 404) setReplayState({ kind: 'expired' })
      else setReplayState({ kind: 'error', message: e instanceof ApiError ? e.message : 'Could not load the replay.' })
    }
  }, [])

  useEffect(() => {
    aliveRef.current = true
    void api<MatchView>(`/tanks/matches/${id}`)
      .then((m) => {
        if (!aliveRef.current) return
        setMatch(m)
        if (m.has_replay) void loadReplay(id)
        else setReplayState({ kind: 'expired' })
      })
      .catch((e) => {
        if (!aliveRef.current) return
        setError(e instanceof ApiError && e.status === 404 ? 'There is no match with this id.' : (e as ApiError).message)
      })
    return () => {
      aliveRef.current = false
    }
  }, [id, loadReplay])

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(window.location.href)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {}
  }

  if (error) {
    return (
      <div className="mx-auto max-w-3xl space-y-4 px-4 py-14 sm:px-6">
        <p role="alert" className="text-destructive">{error}</p>
        <Button variant="outline" render={<Link href="/tanks" />} nativeButton={false}>
          <ArrowLeft />Back to tanks
        </Button>
      </div>
    )
  }
  if (!match) {
    return (
      <div className="mx-auto max-w-5xl space-y-6 px-4 py-14 sm:px-6">
        <Skeleton className="h-10 w-72" />
        <Skeleton className="aspect-[3/2] w-full rounded-[18px]" />
      </div>
    )
  }

  const ranked = [...match.players].sort((a, b) => (a.place ?? 99) - (b.place ?? 99))
  return (
    <div className="mx-auto max-w-5xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader
        kicker={
          <Link href="/tanks" className="inline-flex items-center gap-1.5 hover:text-foreground">
            <ArrowLeft className="size-4" />Tanks
          </Link>
        }
        title={ranked.map((p) => p.name).join(' vs ')}
        actions={
          <Button variant="outline" onClick={copyLink}>
            {copied ? <Check /> : <Copy />}
            {copied ? 'Copied' : 'Copy link'}
          </Button>
        }
      >
        <span className="font-mono text-sm">{match.map} · seed {match.seed} · {match.kind}</span>
      </PageHeader>

      <div className="mt-8">
        {replayState.kind === 'ready' ? (
          <ReplayPlayer replay={replayState.replay} />
        ) : replayState.kind === 'loading' ? (
          <Skeleton className="aspect-[3/2] w-full rounded-[18px]" />
        ) : replayState.kind === 'unsupported' ? (
          <div className="rounded-[18px] border border-dashed border-input px-6 py-14 text-center text-sm text-muted-foreground">
            Your browser can&apos;t open replays here — it&apos;s missing gzip decompression support. Try a
            recent version of Chrome, Firefox, Safari or Edge.
          </div>
        ) : replayState.kind === 'error' ? (
          <div className="flex flex-col items-center gap-4 rounded-[18px] border border-dashed border-input px-6 py-14 text-center">
            <p className="text-sm text-destructive">Couldn&apos;t load the replay: {replayState.message}</p>
            <Button variant="outline" onClick={() => void loadReplay(id)}>Retry</Button>
          </div>
        ) : (
          <div className="rounded-[18px] border border-dashed border-input px-6 py-14 text-center text-sm text-muted-foreground">
            Replay expired. Replays are kept for 3 days unless the match is featured.
          </div>
        )}
      </div>

      <section className="mt-10">
        <SectionTitle>Result</SectionTitle>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Place</TableHead>
              <TableHead>Bot</TableHead>
              <TableHead className="text-right">Kills</TableHead>
              <TableHead className="text-right">Damage</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Rating</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {ranked.map((p) => (
              <TableRow key={p.slot}>
                <TableCell className="font-mono">{p.place ?? '—'}</TableCell>
                <TableCell>
                  <Link href={`/tanks/bots/${p.bot_id}`} className="inline-flex items-center gap-2 font-semibold hover:text-primary">
                    {p.name}
                    <BotBadge source={p.source} house={p.house} />
                  </Link>
                </TableCell>
                <TableCell className="text-right font-mono">{p.kills}</TableCell>
                <TableCell className="text-right font-mono">{p.damage}</TableCell>
                <TableCell className="text-muted-foreground">{p.status}</TableCell>
                <TableCell className="text-right font-mono">{ratingDelta(p.rating_before, p.rating_after)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </section>
    </div>
  )
}
