'use client'

import Link from 'next/link'
import { useCallback, useEffect, useState } from 'react'
import { Trophy } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { AdminBar } from '@/components/products/admin-bar'
import { OpensCountdown, VoteCta } from '@/components/products/next-up'
import { PhaseBadge, friendly, until, untilBare, usePT, utc, utcDay } from '@/components/products/phase'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { products } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { ProductList, ProductTask } from '@/lib/types'

export default function ProductsPage() {
  const t = usePT()
  const when = (iso: string) => utc(t.locale, iso)
  const day = (iso: string) => utcDay(t.locale, iso)
  const kindLabel = (k: ProductTask) => (k.kind === 'site' ? t('kindSite') : t('kindCli', { n: k.scenario_count }))
  const { me } = useMe()
  const [list, setList] = useState<ProductList | null>(null)
  const [error, setError] = useState<string | null>(null)
  const refresh = useCallback(() => {
    products.list().then(setList).catch((e) => setError(friendly(t, e)))
  }, [t])
  useEffect(() => { refresh() }, [refresh])

  const open = list?.items.filter((t) => t.phase === 'open') ?? []
  const voting = list?.items.filter((t) => t.phase === 'voting') ?? []
  const archive = list?.items.filter((t) => t.phase === 'final') ?? []
  const next = list?.upcoming
  // With nothing open the first voting task is the big call to action, so the cards below skip it.
  const votingRest = open.length === 0 ? voting.slice(1) : voting

  return (
    <div className="space-y-8">
      <PageHeader title={t('list.title')}>{t('list.intro')}</PageHeader>
      {me?.can_admin && <AdminBar task={open[0] ?? voting[0]} onChange={refresh} />}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!list && !error && <Skeleton className="h-40 rounded-[14px]" />}

      {list && open.length === 0 && (
        <>
          {voting[0] && <VoteCta task={voting[0]} signedIn={!!me} />}
          {next ? <OpensCountdown upcoming={next} /> : null}
          {!voting[0] && !next?.next_kind && (
            <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">{t('list.noneOpen')}</p>
          )}
        </>
      )}
      {open.map((k) => (
        <section key={k.slug}>
          <SectionTitle aside={t('list.closes', { when: until(t, k.deadline) })}>{t('list.thisWeek')}</SectionTitle>
          <div className="rounded-[14px] border border-primary/40 bg-card p-6">
            <div className="flex flex-wrap items-center gap-2">
              <PhaseBadge phase={k.phase} />
              <span className="text-xs text-muted-foreground">{kindLabel(k)} · {t('list.soFar', { count: t.plural('entries', k.entry_count) })}</span>
            </div>
            <h2 className="display mt-3 text-2xl break-words sm:text-3xl">{k.title}</h2>
            <p className="mt-2 max-w-2xl text-sm text-muted-foreground">{k.summary}</p>
            <p className="mt-4 text-sm"><b>{untilBare(t, k.deadline)}</b> <span className="text-muted-foreground">{t('list.left', { when: when(k.deadline) })}</span></p>
            <Button className="mt-4" render={<Link href={`/products/${k.slug}`} />} nativeButton={false}>{t('list.readAndUpload')}</Button>
          </div>
        </section>
      ))}

      {votingRest.length > 0 && (
        <section>
          <SectionTitle>{open.length === 0 ? t('list.alsoVoting') : t('list.votingNow')}</SectionTitle>
          <div className="grid gap-4 sm:grid-cols-2">
            {votingRest.map((k) => (
              <Link key={k.slug} href={`/products/${k.slug}`}
                className="block min-w-0 rounded-[14px] border border-border bg-card p-5 transition-colors hover:bg-muted/50">
                <div className="flex flex-wrap items-center gap-2">
                  <PhaseBadge phase={k.phase} />
                  <span className="text-xs text-muted-foreground">{t.plural('entries', k.entry_count)}</span>
                </div>
                <h3 className="heading mt-3 text-lg break-words">{k.title}</h3>
                <p className="mt-3 text-xs text-muted-foreground">{t('list.votingEnds', { when: when(k.voting_ends_at), left: until(t, k.voting_ends_at) })}</p>
              </Link>
            ))}
          </div>
        </section>
      )}

      {open.length > 0 && next && next.count > 0 && (
        <p className="text-sm text-muted-foreground">
          {t('list.nextWeek', { kind: next.next_kind === 'cli' ? t('nextKindCli') : t('nextKindSite'), when: when(next.next_opens_at) })}
          {next.count > 1 && t('list.queue', { n: next.count })}
        </p>
      )}

      {list && (
        <section>
          <SectionTitle>{t('list.archive')}</SectionTitle>
          {archive.length === 0 ? (
            <p className="rounded-[14px] border border-dashed border-input px-5 py-8 text-center text-sm text-muted-foreground">{t('list.noFinished')}</p>
          ) : (
            <ul className="space-y-2">
              {archive.map((k) => (
                <li key={k.slug}>
                  <Link href={`/products/${k.slug}/results`}
                    className="flex min-w-0 flex-col gap-1 rounded-[14px] border border-border bg-card px-5 py-4 transition-colors hover:bg-muted/50 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
                    <div className="min-w-0">
                      <p className="text-xs text-muted-foreground">{t('list.weekOf', { day: day(k.opens_at), kind: k.kind === 'site' ? t('kindSite') : t('kindTool'), count: t.plural('entries', k.entry_count) })}</p>
                      <h3 className="heading mt-0.5 text-base break-words">{k.title}</h3>
                    </div>
                    <p className="inline-flex shrink-0 items-center gap-1.5 text-sm">
                      <Trophy className="size-4 text-muted-foreground" />
                      {k.winner
                        ? <><b className="break-all">{k.winner.handle}</b><span className="text-muted-foreground">{k.kind === 'cli' ? ` · ${k.winner.passed}/${k.winner.total}` : ''} · {t.plural('votes', k.winner.votes)}</span></>
                        : <span className="text-muted-foreground">{t('list.noWinner')}</span>}
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
