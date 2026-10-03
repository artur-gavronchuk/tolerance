'use client'

import { useEffect, useState } from 'react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { StartTournament } from '@/components/tanks/start-tournament'
import { TournamentList } from '@/components/tanks/tournament-list'
import { Skeleton } from '@/components/ui/skeleton'
import { tanks } from '@/lib/api'
import { useT } from '@/lib/i18n/client'
import { tanksHomeMessages as m } from '@/lib/i18n/messages/tanks-home'
import type { TournamentView } from '@/lib/types'

export default function TournamentsPage() {
  const tr = useT(m)
  const [items, setItems] = useState<TournamentView[] | null>(null)

  useEffect(() => {
    const load = () =>
      void tanks
        .tournaments()
        .then((r) => setItems(r.items))
        .catch(() => setItems((cur) => cur ?? []))
    load()
    const t = setInterval(load, 10000)
    return () => clearInterval(t)
  }, [])

  const upcoming = (items ?? []).filter((t) => t.status === 'scheduled' || t.status === 'running')
  const past = (items ?? []).filter((t) => t.status === 'finished')

  return (
    <div className="mx-auto max-w-4xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader title={tr('tours.title')} actions={<StartTournament />}>
        {tr('tours.intro')}
      </PageHeader>

      {items == null ? (
        <Skeleton className="mt-8 h-64 rounded-[14px]" />
      ) : (
        <>
          <section className="mt-8">
            <SectionTitle>{tr('tours.upcoming')}</SectionTitle>
            <TournamentList items={upcoming} empty={tr('tours.nothing')} />
          </section>
          <section className="mt-10">
            <SectionTitle>{tr('tours.past')}</SectionTitle>
            <TournamentList items={past} empty={tr('home.noFinished')} />
          </section>
        </>
      )}
    </div>
  )
}
