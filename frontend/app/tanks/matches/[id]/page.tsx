'use client'

import { use, useEffect, useState } from 'react'
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

export default function MatchPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params)
  const [match, setMatch] = useState<MatchView | null>(null)
  const [replay, setReplay] = useState<Replay | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    let cancelled = false
    void api<MatchView>(`/tanks/matches/${id}`)
      .then((m) => {
        if (cancelled) return
        setMatch(m)
        if (m.has_replay) {
          void fetchReplay(id)
            .then((r) => { if (!cancelled) setReplay(r) })
            .catch(() => {})
        }
      })
      .catch((e) => setError((e as ApiError).status === 404 ? 'There is no match with this id.' : (e as ApiError).message))
    return () => {
      cancelled = true
    }
  }, [id])

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
        {replay ? (
          <ReplayPlayer replay={replay} />
        ) : match.has_replay ? (
          <Skeleton className="aspect-[3/2] w-full rounded-[18px]" />
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
