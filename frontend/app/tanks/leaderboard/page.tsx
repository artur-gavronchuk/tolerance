'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { PageHeader } from '@/components/page-header'
import { Countdown } from '@/components/tanks/countdown'
import { Leaderboard } from '@/components/tanks/leaderboard'
import { Skeleton } from '@/components/ui/skeleton'
import { tanks } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { SeasonDetail, SeasonView } from '@/lib/types'

// The ladder is the current season. Archived seasons live at /tanks/seasons/{id}; the picker links to them.
export default function LadderPage() {
  const [d, setD] = useState<SeasonDetail | null>(null)
  const [all, setAll] = useState<SeasonView[]>([])
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    void tanks.season('current').then(setD).catch(() => setFailed(true))
    void tanks.seasons().then(setAll).catch(() => {})
  }, [])

  const s = d?.season
  return (
    <div className="mx-auto max-w-4xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader title="Season ladder">
        {d && s ? (
          <>
            Season {s.name}, ends in <Countdown to={s.ends_at} serverNow={d.now} />. Every active bot, ranked by a conservative
            estimate of its skill. Ratings start fresh every month.
          </>
        ) : (
          <>Every active bot, ranked by a conservative estimate of its skill in the current season. Ratings start fresh every month.</>
        )}
      </PageHeader>

      {all.length > 1 && (
        <div className="mt-6 flex flex-wrap items-center gap-2" aria-label="Seasons">
          {all.map((x) => (
            <Link
              key={x.id}
              href={x.status === 'active' ? '/tanks/leaderboard' : `/tanks/seasons/${x.id}`}
              aria-current={x.id === s?.id ? 'page' : undefined}
              className={cn(
                'rounded-full border border-border px-3 py-1.5 text-sm font-semibold text-muted-foreground hover:text-foreground',
                x.id === s?.id && 'bg-muted text-foreground',
              )}
            >
              {x.name}
              {x.status === 'active' && ' (current)'}
            </Link>
          ))}
        </div>
      )}

      <div className="mt-8">
        {failed ? (
          <p role="alert" className="text-sm text-destructive">Could not load the ladder.</p>
        ) : d == null ? (
          <Skeleton className="h-96 rounded-[14px]" />
        ) : (
          <Leaderboard entries={d.standings} />
        )}
      </div>
    </div>
  )
}
