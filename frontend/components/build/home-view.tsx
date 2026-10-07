'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Heart, Lock, Trophy } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { ScoreBadge, ScoreSplit, Thumb } from '@/components/build/build-gallery'
import { api } from '@/lib/api'
import { FORMATS, daysLeft, tx } from '@/lib/build-kinds'
import { errorText } from '@/lib/format'
import { formatDate } from '@/lib/i18n/core'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildCard, BuildEntry } from '@/lib/types'
import { cn } from '@/lib/utils'

// The home page: what this is in two lines, then the season's challenges as posters, in season order.
export function HomeView() {
  const t = useT(buildMessages)
  const [cards, setCards] = useState<BuildCard[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    api<{ items: BuildCard[] }>('/builds').then((r) => setCards(r.items), (e) => setError(errorText(e, t.locale)))
  }, [t.locale])

  return (
    <div className="space-y-12 sm:space-y-16">
      <section className="grid gap-10 pt-2 lg:grid-cols-[minmax(0,1fr)_21rem] lg:items-end lg:gap-16">
        <div>
          <h1 className="poster max-w-3xl text-[2.5rem] sm:text-[4.6rem]">{t('homeTitle')}</h1>
          <p className="mt-6 max-w-2xl text-[15px] leading-relaxed text-muted-foreground sm:text-lg">{t('homeText')}</p>
        </div>
        <div className="space-y-6">
          <ol className="space-y-3">
            {(['homeStep1', 'homeStep2', 'homeStep3'] as const).map((k, i) => (
              <li key={k} className="flex items-center gap-3 font-semibold">
                <span className="poster flex size-8 shrink-0 items-center justify-center rounded-full bg-ink text-sm text-ink-foreground">{i + 1}</span>
                {t(k)}
              </li>
            ))}
          </ol>
          <ScoreSplit legend />
        </div>
      </section>

      {error && <p role="alert" className="text-destructive">{error}</p>}
      <section className="grid gap-5 md:grid-cols-2 lg:gap-6">
        {cards === null
          ? [0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-[34rem] rounded-[28px]" />)
          : cards.map((c) => <Poster key={c.slug} c={c} />)}
      </section>
    </div>
  )
}

// Poster is one challenge: a flat format colour, the title set large and the best work rising from the bottom edge.
function Poster({ c }: { c: BuildCard }) {
  const t = useT(buildMessages)
  const f = FORMATS[c.format]
  const date = (iso: string) => formatDate(t.locale, iso, { day: 'numeric', month: 'long' })
  const left = daysLeft(c.closes_at)

  if (c.status === 'upcoming') {
    return (
      <Link href={`/c/${encodeURIComponent(c.slug)}`}
        className="flex min-h-[24rem] flex-col rounded-[28px] border-2 md:min-h-[34rem] border-dashed border-strong p-6 text-muted-foreground transition-colors hover:border-input sm:p-8">
        <span className="flex items-center gap-1.5 text-sm font-bold"><f.icon className="size-4" />{t(`format_${c.format}`)}</span>
        <h2 className="poster mt-6 text-[2.2rem] sm:text-[3.2rem]">{tx(c.title, t.locale)}</h2>
        <p className="mt-3 max-w-sm text-[15px] leading-snug">{tx(c.one_liner, t.locale)}</p>
        <span className="mt-auto flex items-center gap-2 font-bold"><Lock className="size-4" />{t('opensOn', { date: date(c.opens_at) })}</span>
      </Link>
    )
  }

  return (
    <Link href={`/c/${encodeURIComponent(c.slug)}`}
      className={cn('group relative flex flex-col md:min-h-[34rem] overflow-hidden rounded-[28px] text-terminal', f.bg)}>
      <div className="relative z-10 p-6 sm:p-8">
        <div className="flex flex-wrap items-center justify-between gap-2 text-sm font-bold">
          <span className="flex items-center gap-1.5"><f.icon className="size-4" />{t(`format_${c.format}`)}</span>
          <span>{c.status === 'closed' ? t('closedOn', { date: date(c.closes_at) }) : left > 1 ? t.plural('daysLeft', left) : t('lastDay')}</span>
        </div>
        <h2 className="poster mt-6 text-[2.2rem] sm:text-[3.2rem]">{tx(c.title, t.locale)}</h2>
        <p className="mt-3 max-w-sm text-[15px] font-medium leading-snug opacity-80">{tx(c.one_liner, t.locale)}</p>
        <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2 text-sm font-bold">
          <span className="flex items-center gap-1.5"><Trophy className="size-4" />{t.plural('solutions', c.entries)}</span>
          <span className="flex items-center gap-1.5"><Heart className="size-4" />{t.plural('votes', c.votes)}</span>
          {c.mine && (
            <span className="rounded-full bg-terminal px-2.5 py-0.5 text-xs text-terminal-foreground">
              {c.mine.score !== null ? t('yourEntryScore', { score: c.mine.score }) : t('yourEntryPending')}
            </span>
          )}
        </div>
      </div>
      <Exhibit entries={c.top} mobile={c.format === 'mobile'} />
    </Link>
  )
}

// Exhibit is the best work (the reference while there are no entries) cut off by the poster's bottom edge,
// with the next two peeking out behind it.
function Exhibit({ entries, mobile }: { entries: BuildEntry[]; mobile: boolean }) {
  const t = useT(buildMessages)
  if (entries.length === 0) {
    return (
      <div className="mx-6 mb-6 mt-auto flex h-40 items-center justify-center rounded-[18px] border-2 border-dashed border-terminal/30 px-4 text-center font-bold sm:mx-8 sm:mb-8">
        {t('beFirst')}
      </div>
    )
  }
  const [first, ...rest] = entries
  const frame = mobile
    ? 'w-44 aspect-[390/720] rounded-t-[30px] border-[6px] border-b-0 sm:w-48'
    : 'w-[84%] aspect-[16/10] rounded-t-[16px] border-[5px] border-b-0'
  // Each frame hangs from the top of this strip; the poster's own edge cuts it off at the bottom.
  return (
    <div className={cn('relative mt-auto', mobile ? 'h-60 sm:h-64' : 'h-56 sm:h-64')}>
      {rest.slice(0, 2).map((e, i) => (
        <div key={e.id} aria-hidden className="absolute inset-x-0 top-10 flex justify-center">
          <div className={cn('overflow-hidden border-terminal bg-terminal opacity-90', frame,
            i === 0 ? '-translate-x-[18%] -rotate-6' : 'translate-x-[18%] rotate-6')}>
            <Thumb e={e} />
          </div>
        </div>
      ))}
      <div className="absolute inset-x-0 top-4 flex justify-center transition-transform duration-300 group-hover:-translate-y-2">
        <div className={cn('relative overflow-hidden border-terminal bg-terminal shadow-2xl', frame)}>
          <Thumb e={first} />
          <span className="absolute right-2 top-2"><ScoreBadge score={first.reference ? first.test_score : first.score} max={first.reference ? 60 : 100} /></span>
        </div>
      </div>
    </div>
  )
}
