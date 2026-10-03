'use client'

import Link from 'next/link'
import { use } from 'react'
import { Download } from 'lucide-react'
import { HandleLink } from '@/components/daily/handle-link'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Podium } from '@/components/products/podium'
import { PhaseBadge, VotingNote, rankRuleText } from '@/components/products/phase'
import { useResults } from '@/components/products/use-results'
import { VoteButton } from '@/components/products/vote-button'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { useMe } from '@/lib/use-me'

export default function ProductResultsPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = use(params)
  const { me } = useMe()
  const { res, error, voteError, busy, toggleVote } = useResults(slug, me?.user.handle)

  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!res) return <Skeleton className="h-64 rounded-[14px]" />
  const { task, entries } = res
  const scored = entries.some((e) => e.total > 0) // sites without automated checks have no score column

  return (
    <div className="space-y-8">
      <PageHeader
        kicker={<Link href={`/products/${slug}`} className="hover:text-foreground">{task.title}</Link>}
        title="Results"
        actions={task.phase !== 'open' && <Button variant="outline" render={<Link href={`/products/${slug}`} />} nativeButton={false}>Gallery and voting</Button>}
      >
        <span className="mr-2 inline-block align-middle"><PhaseBadge phase={task.phase} /></span>
        {task.phase === 'open'
          ? `Entries are published after the deadline, ${new Date(task.deadline).toLocaleString()}. ${task.entry_count} submitted so far.`
          : rankRuleText(task.kind, scored)}
      </PageHeader>
      <VotingNote task={task} />
      {voteError && <p role="alert" className="text-sm text-destructive">{voteError}</p>}
      {task.phase !== 'open' && entries.length === 0 && (
        <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">Nobody entered this task.</p>
      )}
      {entries.length > 0 && (
        <>
          <section>
            <SectionTitle aside={task.phase === 'final' ? 'Final' : 'Provisional until voting ends'}>Podium</SectionTitle>
            <Podium task={task} entries={entries} />
          </section>
          <section>
            <SectionTitle>All entries</SectionTitle>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">#</TableHead>
                  <TableHead>Player</TableHead>
                  {scored && <TableHead className="text-right">{task.kind === 'site' ? 'Checks' : 'Score'}</TableHead>}
                  <TableHead className="text-right">Votes</TableHead>
                  <TableHead className="w-24" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {entries.map((e, i) => (
                  <TableRow key={e.id} className={e.mine ? 'bg-accent' : undefined}>
                    <TableCell className="font-mono text-muted-foreground">{i + 1}</TableCell>
                    <TableCell className="max-w-[8rem] min-w-0 sm:max-w-none">
                      <div className="truncate font-semibold">{e.handle ? <HandleLink handle={e.handle} /> : '?'}</div>
                      {e.made_with && <div className="truncate text-xs text-muted-foreground">{e.made_with}</div>}
                    </TableCell>
                    {scored && <TableCell className="text-right font-mono font-bold">{e.total > 0 ? `${e.passed}/${e.total}` : '-'}</TableCell>}
                    <TableCell className="text-right font-mono">{e.votes}</TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        {task.phase === 'voting' && !e.mine && (
                          <VoteButton compact entry={e} phase={task.phase} signedIn={!!me} busy={busy === e.id} onToggle={() => void toggleVote(e)} />
                        )}
                        <Button size="icon" variant="ghost" aria-label="Download source" title="Download source"
                          render={<a href={`/api/v1/product-entries/${e.id}/zip`} />} nativeButton={false}><Download /></Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </section>
        </>
      )}
    </div>
  )
}
