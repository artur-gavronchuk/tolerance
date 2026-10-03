'use client'

import Link from 'next/link'
import { useCallback, useEffect, useState } from 'react'
import { Trophy } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { AdminBar } from '@/components/products/admin-bar'
import { OpensCountdown, VoteCta } from '@/components/products/next-up'
import { PhaseBadge, until, utc, utcDay } from '@/components/products/phase'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { friendlyMessage, products } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { ProductList, ProductTask } from '@/lib/types'

const when = utc
const day = utcDay
const kindLabel = (t: ProductTask) => (t.kind === 'site' ? 'Website' : `Command-line tool, ${t.scenario_count} scenarios`)

export default function ProductsPage() {
  const { me } = useMe()
  const [list, setList] = useState<ProductList | null>(null)
  const [error, setError] = useState<string | null>(null)
  const refresh = useCallback(() => {
    products.list().then(setList).catch((e) => setError(friendlyMessage(e)))
  }, [])
  useEffect(() => { refresh() }, [refresh])

  const open = list?.items.filter((t) => t.phase === 'open') ?? []
  const voting = list?.items.filter((t) => t.phase === 'voting') ?? []
  const archive = list?.items.filter((t) => t.phase === 'final') ?? []
  const next = list?.upcoming

  return (
    <div className="space-y-8">
      <PageHeader title="Product of the week">
        One product to build with your own agent each week, from Monday to Monday (UTC). Upload the result before the deadline; tools are
        scored by automated scenarios, sites are shown to everyone, and after the deadline everyone votes on the published entries.
      </PageHeader>
      {me?.can_admin && <AdminBar task={open[0] ?? voting[0]} onChange={refresh} />}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!list && !error && <Skeleton className="h-40 rounded-[14px]" />}

      {list && open.length === 0 && (
        <>
          {voting[0] && <VoteCta task={voting[0]} signedIn={!!me} />}
          {next ? <OpensCountdown upcoming={next} /> : null}
          {!voting[0] && !next?.next_kind && (
            <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">No task is open right now.</p>
          )}
        </>
      )}
      {open.map((t) => (
        <section key={t.slug}>
          <SectionTitle aside={`Closes ${until(t.deadline)}`}>This week</SectionTitle>
          <div className="rounded-[14px] border border-primary/40 bg-card p-6">
            <div className="flex flex-wrap items-center gap-2">
              <PhaseBadge phase={t.phase} />
              <span className="text-xs text-muted-foreground">{kindLabel(t)} · {t.entry_count} {t.entry_count === 1 ? 'entry' : 'entries'} so far</span>
            </div>
            <h2 className="display mt-3 text-2xl break-words sm:text-3xl">{t.title}</h2>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">{t.summary}</p>
            <p className="mt-4 text-sm"><b>{until(t.deadline).replace('in ', '')}</b> <span className="text-muted-foreground">left, uploads close {when(t.deadline)}</span></p>
            <Button className="mt-4" render={<Link href={`/products/${t.slug}`} />} nativeButton={false}>Read the task and upload</Button>
          </div>
        </section>
      ))}

      {voting.length > 0 && (
        <section>
          <SectionTitle>Voting now</SectionTitle>
          <div className="grid gap-4 sm:grid-cols-2">
            {voting.map((t) => (
              <Link key={t.slug} href={`/products/${t.slug}`}
                className="block min-w-0 rounded-[14px] border border-border bg-card p-5 transition-colors hover:bg-muted/50">
                <div className="flex flex-wrap items-center gap-2">
                  <PhaseBadge phase={t.phase} />
                  <span className="text-xs text-muted-foreground">{t.entry_count} {t.entry_count === 1 ? 'entry' : 'entries'}</span>
                </div>
                <h3 className="heading mt-3 text-lg break-words">{t.title}</h3>
                <p className="mt-3 text-xs text-muted-foreground">Voting ends {when(t.voting_ends_at)} ({until(t.voting_ends_at)})</p>
              </Link>
            ))}
          </div>
        </section>
      )}

      {open.length > 0 && next && next.count > 0 && (
        <p className="text-sm text-muted-foreground">
          Next week: {next.next_kind === 'cli' ? 'a command-line tool' : 'a website'}, opens {when(next.next_opens_at)}.
          {next.count > 1 && ` ${next.count} tasks are waiting in the queue.`}
        </p>
      )}

      {list && (
        <section>
          <SectionTitle>Archive</SectionTitle>
          {archive.length === 0 ? (
            <p className="rounded-[14px] border border-dashed border-input px-5 py-8 text-center text-sm text-muted-foreground">No finished weeks yet.</p>
          ) : (
            <ul className="space-y-2">
              {archive.map((t) => (
                <li key={t.slug}>
                  <Link href={`/products/${t.slug}/results`}
                    className="flex min-w-0 flex-col gap-1 rounded-[14px] border border-border bg-card px-5 py-4 transition-colors hover:bg-muted/50 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
                    <div className="min-w-0">
                      <p className="text-xs text-muted-foreground">Week of {day(t.opens_at)} · {t.kind === 'site' ? 'Website' : 'Tool'} · {t.entry_count} {t.entry_count === 1 ? 'entry' : 'entries'}</p>
                      <h3 className="heading mt-0.5 text-base break-words">{t.title}</h3>
                    </div>
                    <p className="inline-flex shrink-0 items-center gap-1.5 text-sm">
                      <Trophy className="size-4 text-muted-foreground" />
                      {t.winner
                        ? <><b className="break-all">{t.winner.handle}</b><span className="text-muted-foreground">{t.kind === 'cli' ? ` · ${t.winner.passed}/${t.winner.total}` : ''} · {t.winner.votes} {t.winner.votes === 1 ? 'vote' : 'votes'}</span></>
                        : <span className="text-muted-foreground">No winner</span>}
                    </p>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </div>
  )
}
