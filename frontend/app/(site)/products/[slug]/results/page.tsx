'use client'

import Link from 'next/link'
import { use, useCallback, useEffect, useState } from 'react'
import { Download, ThumbsUp } from 'lucide-react'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { friendlyMessage, products } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { SitePreview } from '@/components/products/entry-card'
import type { ProductResults } from '@/lib/types'

export default function ProductResultsPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = use(params)
  const { me } = useMe()
  const [res, setRes] = useState<ProductResults | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [voteError, setVoteError] = useState<string | null>(null)
  const refresh = useCallback(() => {
    products.results(slug).then(setRes).catch((e) => setError(friendlyMessage(e)))
  }, [slug])
  useEffect(() => { refresh() }, [refresh, me?.user.handle])

  async function vote(id: string) {
    setVoteError(null)
    try {
      await products.vote(id)
      refresh()
    } catch (e) {
      setVoteError(friendlyMessage(e))
    }
  }

  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!res) return <Skeleton className="h-64 rounded-[14px]" />
  const { task, entries } = res

  return (
    <div className="space-y-8">
      <PageHeader kicker={<Link href={`/products/${slug}`} className="hover:text-foreground">{task.title}</Link>} title="Results">
        {task.phase === 'open'
          ? `Entries are published after the deadline, ${new Date(task.deadline).toLocaleString()}. ${task.entry_count} scored so far.`
          : task.kind === 'site'
            ? 'Ranked by votes (automated checks, where the task has them, break ties). Try each site and vote once per entry, not for your own.'
            : 'Ranked by scenarios passed, then votes. You can vote once per entry, not for your own.'}
      </PageHeader>
      {voteError && <p role="alert" className="text-sm text-destructive">{voteError}</p>}
      {task.phase === 'voting' && entries.length === 0 && (
        <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">Nobody entered this task.</p>
      )}
      {entries.length > 0 && task.kind === 'site' && (
        <ul className="grid gap-5 md:grid-cols-2">
          {entries.map((e, i) => (
            <li key={e.id} className={`min-w-0 space-y-3 rounded-[14px] border bg-card p-4 ${e.mine ? 'border-primary' : 'border-border'}`}>
              <SitePreview id={e.id} title={`${e.handle}'s site`} />
              <div className="flex items-center gap-3">
                <span className="font-mono text-sm text-muted-foreground">#{i + 1}</span>
                <div className="min-w-0 flex-1">
                  <div className="truncate font-semibold">{e.handle}{e.mine && <span className="font-normal text-muted-foreground"> (you)</span>}</div>
                  {e.made_with && <div className="truncate text-xs text-muted-foreground">{e.made_with}</div>}
                </div>
                {e.total > 0 && <span className="font-mono text-xs text-muted-foreground" title="Automated checks passed">{e.passed}/{e.total}</span>}
                <Button size="sm" variant={e.voted ? 'secondary' : 'outline'} disabled={!me || e.mine || e.voted}
                  aria-label={e.voted ? 'Voted' : 'Vote'} onClick={() => void vote(e.id)}>
                  <ThumbsUp />{e.votes}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {entries.length > 0 && task.kind !== 'site' && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-10">#</TableHead>
              <TableHead>Player</TableHead>
              <TableHead className="text-right">Score</TableHead>
              <TableHead className="text-right">Votes</TableHead>
              <TableHead className="w-24" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {entries.map((e, i) => (
              <TableRow key={e.id} className={e.mine ? 'bg-accent' : undefined}>
                <TableCell className="font-mono text-muted-foreground">{i + 1}</TableCell>
                <TableCell className="max-w-[8rem] min-w-0 sm:max-w-none">
                  <div className="truncate font-semibold">{e.handle}</div>
                  {e.made_with && <div className="truncate text-xs text-muted-foreground">{e.made_with}</div>}
                </TableCell>
                <TableCell className="text-right font-mono font-bold">{e.passed}/{e.total}</TableCell>
                <TableCell className="text-right font-mono">{e.votes}</TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-1">
                    {me && !e.mine && (
                      <Button size="icon" variant={e.voted ? 'secondary' : 'outline'} disabled={e.voted} aria-label={e.voted ? 'Voted' : 'Vote'}
                        title={e.voted ? 'Voted' : 'Vote'} onClick={() => void vote(e.id)}><ThumbsUp /></Button>
                    )}
                    <Button size="icon" variant="ghost" aria-label="Download source" title="Download source"
                      render={<a href={`/api/v1/product-entries/${e.id}/zip`} />} nativeButton={false}><Download /></Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      {task.phase === 'voting' && !me && (
        <p className="text-sm text-muted-foreground"><Link className="font-semibold text-primary hover:underline" href="/login">Sign in</Link> to vote.</p>
      )}
    </div>
  )
}
