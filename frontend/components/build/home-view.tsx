'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowRight, CalendarClock, Heart, Lock, Trophy } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { ScoreBadge, shotUrl } from '@/components/build/build-gallery'
import { api } from '@/lib/api'
import { FORMATS, daysLeft, tx } from '@/lib/build-kinds'
import { errorText } from '@/lib/format'
import { formatDate } from '@/lib/i18n/core'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildCard, BuildEntry } from '@/lib/types'
import { cn } from '@/lib/utils'

// The home page: the season's challenges, 2×2, in season order. The open one is the bright one.
export function HomeView() {
  const t = useT(buildMessages)
  const [cards, setCards] = useState<BuildCard[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api<{ items: BuildCard[] }>('/builds').then((r) => setCards(r.items), (e) => setError(errorText(e, t.locale)))
  }, [t.locale])

  return (
    <div className="space-y-10">
      <section className="relative overflow-hidden rounded-[24px] bg-terminal px-5 py-9 text-terminal-foreground sm:px-10 sm:py-12">
        <div aria-hidden className="pointer-events-none absolute -right-20 -top-28 size-96 rounded-full bg-pop-2/25 blur-3xl" />
        <div aria-hidden className="pointer-events-none absolute -bottom-40 left-10 size-96 rounded-full bg-pop-4/20 blur-3xl" />
        <div aria-hidden className="pointer-events-none absolute -bottom-24 right-1/3 size-72 rounded-full bg-pop-1/25 blur-3xl" />
        <div className="relative max-w-3xl">
          <span className="rounded-full bg-gradient-to-r from-pop-1 via-pop-2 to-pop-3 px-3 py-1 text-[13px] font-bold text-white">{t('homeKicker')}</span>
          <h1 className="display mt-5 text-[2.3rem] sm:text-[3.4rem]">
            <span className="bg-gradient-to-r from-pop-4 via-pop-1 to-pop-2 bg-clip-text text-transparent">{t('homeTitle')}</span>
          </h1>
          <p className="mt-4 text-[15px] leading-relaxed text-terminal-foreground/80 sm:text-lg">{t('homeText')}</p>
          <ol className="mt-6 flex flex-wrap gap-2 text-sm font-bold">
            {(['homeStep1', 'homeStep2', 'homeStep3'] as const).map((k, i) => (
              <li key={k} className="flex items-center gap-2 rounded-full border border-white/15 bg-white/[0.06] py-1.5 pl-1.5 pr-3.5">
                <span className="flex size-6 items-center justify-center rounded-full bg-white/15 font-mono text-xs">{i + 1}</span>{t(k)}
              </li>
            ))}
          </ol>
        </div>
      </section>

      {error && <p role="alert" className="text-destructive">{error}</p>}
      <section className="grid gap-6 md:grid-cols-2">
        {cards === null
          ? [0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-[28rem] rounded-[22px]" />)
          : cards.map((c) => <ChallengeCard key={c.slug} c={c} />)}
      </section>
    </div>
  )
}

function ChallengeCard({ c }: { c: BuildCard }) {
  const t = useT(buildMessages)
  const f = FORMATS[c.format]
  const upcoming = c.status === 'upcoming'
  const date = (iso: string) => formatDate(t.locale, iso, { day: 'numeric', month: 'long' })
  const left = daysLeft(c.closes_at)
  return (
    <Link href={`/c/${encodeURIComponent(c.slug)}`}
      className={cn('group relative flex flex-col overflow-hidden rounded-[22px] border border-border bg-card transition-all',
        upcoming ? 'hover:border-strong' : 'hover:-translate-y-1 hover:shadow-xl')}>
      <div className={cn('relative overflow-hidden px-6 pb-6 pt-5',
        upcoming ? 'bg-muted text-foreground' : cn('bg-gradient-to-br text-white', f.gradient))}>
        <f.icon aria-hidden className={cn('absolute -right-6 -top-6 size-40 rotate-12 transition-transform duration-500',
          upcoming ? 'opacity-[0.07]' : 'opacity-20 group-hover:rotate-0 group-hover:scale-110')} />
        <div className="relative flex flex-wrap items-center gap-2">
          <span className={cn('inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-bold uppercase tracking-wide',
            upcoming ? 'bg-card text-muted-foreground' : 'bg-black/20')}>
            <f.icon className="size-3.5" />{t(`format_${c.format}`)}
          </span>
          {c.status === 'open' && (
            <span className="inline-flex items-center gap-1 rounded-full bg-white px-2.5 py-1 text-xs font-bold text-terminal">
              <CalendarClock className="size-3.5" />{left > 1 ? t.plural('daysLeft', left) : t('lastDay')}
            </span>
          )}
          {c.status === 'closed' && (
            <span className="rounded-full bg-black/20 px-2.5 py-1 text-xs font-bold">{t('closedOn', { date: date(c.closes_at) })}</span>
          )}
        </div>
        <h2 className="display relative mt-4 text-[2rem] sm:text-[2.4rem]">{tx(c.title, t.locale)}</h2>
        <p className={cn('relative mt-2 max-w-md text-[15px] leading-snug', upcoming ? 'text-muted-foreground' : 'text-white/90')}>
          {tx(c.one_liner, t.locale)}
        </p>
      </div>
      <div className="flex flex-1 flex-col gap-4 p-5">
        {upcoming ? (
          <div className="flex flex-1 items-center justify-center gap-2 rounded-[14px] border-2 border-dashed border-strong px-4 py-10 text-center font-bold text-muted-foreground">
            <Lock className="size-4" />{t('opensOn', { date: date(c.opens_at) })}
          </div>
        ) : (
          <>
            <Thumbs entries={c.top} mobile={c.format === 'mobile'} />
            <div className="mt-auto flex flex-wrap items-center gap-x-4 gap-y-2 text-sm">
              <span className="flex items-center gap-1.5 font-bold"><Trophy className="size-4 text-muted-foreground" />{t.plural('solutions', c.entries)}</span>
              <span className="flex items-center gap-1.5 text-muted-foreground"><Heart className="size-4" />{t.plural('votes', c.votes)}</span>
              {c.mine && (
                <span className="rounded-full bg-accent px-2.5 py-0.5 text-xs font-bold text-accent-foreground">
                  {c.mine.score !== null ? t('yourEntryScore', { score: c.mine.score }) : t('yourEntryPending')}
                </span>
              )}
              <span className="ml-auto flex items-center gap-1 font-bold text-primary">
                {t('openChallenge')}<ArrowRight className="size-4 transition-transform group-hover:translate-x-1" />
              </span>
            </div>
          </>
        )}
      </div>
    </Link>
  )
}

// The best three entries as screenshots (the reference fills in while there are fewer), or an invitation.
function Thumbs({ entries, mobile }: { entries: BuildEntry[]; mobile: boolean }) {
  const t = useT(buildMessages)
  if (entries.length === 0) {
    return (
      <div className={cn('flex items-center justify-center rounded-[14px] border-2 border-dashed border-strong px-4 text-center text-sm font-semibold text-muted-foreground',
        mobile ? 'h-48' : 'aspect-[16/7]')}>
        {t('beFirst')}
      </div>
    )
  }
  return (
    <div className="grid grid-cols-3 gap-2">
      {entries.map((e) => (
        <div key={e.id} className={cn('relative overflow-hidden rounded-[10px] border border-border bg-muted', mobile ? 'aspect-[390/600]' : 'aspect-[16/10]')}>
          {e.has_shot
            // eslint-disable-next-line @next/next/no-img-element
            ? <img src={shotUrl(e)} alt="" loading="lazy" className="size-full object-cover object-top" />
            : <span className="flex size-full items-center justify-center font-mono text-xs text-muted-foreground">{e.handle}</span>}
          <span className="absolute left-1.5 top-1.5 flex h-5 min-w-5 items-center justify-center rounded-full bg-card px-1.5 font-mono text-[11px] font-bold shadow">
            {e.reference ? '★' : e.place}
          </span>
          <span className="absolute bottom-1.5 right-1.5"><ScoreBadge score={e.reference ? e.test_score : e.score} /></span>
        </div>
      ))}
    </div>
  )
}
