'use client'

import Link from 'next/link'
import { useEffect, useState } from 'react'
import { products } from '@/lib/api'
import { Button } from '@/components/ui/button'
import type { CompareNext, ProductList, ProductTask } from '@/lib/types'
import { until, usePT, utc } from './phase'

// "in 2d 5h" that keeps ticking.
function useUntil(iso: string) {
  const t = usePT()
  const [, tick] = useState(0)
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 30_000)
    return () => clearInterval(t)
  }, [])
  return until(t, iso)
}

// When no task is open: lead with the one in its voting window.
export function VoteCta({ task, signedIn }: { task: ProductTask; signedIn: boolean }) {
  const t = usePT()
  const [next, setNext] = useState<CompareNext | null>(null)
  const site = task.kind === 'site'
  useEffect(() => {
    if (!site || !signedIn) return
    products.compareNext(task.slug).then(setNext).catch(() => {})
  }, [site, signedIn, task.slug])

  // Only signed-in people have a pair count; everybody else gets a plain label. A target of 0 means there is
  // nothing to compare yet, which is not the same as being done.
  const left = next ? Math.max(0, next.target - next.judged) : 0
  const done = site && next != null && next.target > 0 && left === 0
  const label = done ? t('cta.seeEntries')
    : site && left > 0 ? t('cta.voteJudge', { pairs: t.plural('pairs', left) })
    : t('cta.voteNow')
  const ends = useUntil(task.voting_ends_at)
  return (
    <section className="rounded-[14px] border border-primary/40 bg-card p-6">
      <p className="text-xs text-muted-foreground">{t('cta.votingOpenFor', { title: task.title })}</p>
      <h2 className="display mt-2 text-2xl sm:text-3xl">{done ? t('cta.thanks') : site ? t('cta.whichSite') : t('cta.whichTool')}</h2>
      <p className="mt-2 max-w-2xl text-sm text-muted-foreground">
        {site ? t('cta.siteHelp') : t('cta.toolHelp')}{' '}
        {t('cta.votingEnds', { when: utc(t.locale, task.voting_ends_at), left: ends })}
      </p>
      <Button size="lg" className="mt-4 w-full sm:w-auto" render={<Link href={`/products/${task.slug}`} />} nativeButton={false}>{label}</Button>
    </section>
  )
}

// When no task is open: when the next one starts.
export function OpensCountdown({ upcoming }: { upcoming: ProductList['upcoming'] }) {
  const t = usePT()
  const has = !!upcoming.next_kind && !!upcoming.next_opens_at
  const left = useUntil(has ? upcoming.next_opens_at : new Date().toISOString())
  if (!has) return null
  return (
    <section className="rounded-[14px] border border-border bg-card p-5">
      <p className="text-xs text-muted-foreground">{t('cta.nextProduct', { kind: upcoming.next_kind === 'cli' ? t('nextKindCli') : t('nextKindSite') })}</p>
      <p className="display mt-1 text-2xl">{t('cta.opens', { left })}</p>
      <p className="mt-1 text-sm text-muted-foreground">{utc(t.locale, upcoming.next_opens_at)}</p>
    </section>
  )
}
