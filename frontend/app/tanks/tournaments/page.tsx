'use client'

import { useEffect, useState } from 'react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { StartTournament } from '@/components/tanks/start-tournament'
import { TournamentList } from '@/components/tanks/tournament-list'
import { Skeleton } from '@/components/ui/skeleton'
import { tanks } from '@/lib/api'
import type { TournamentView } from '@/lib/types'

export default function TournamentsPage() {
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
      <PageHeader title="Tournaments" actions={<StartTournament />}>
        Every Saturday at 18:00 UTC the top 8 bots of the season ladder play a single-elimination bracket. Every
        pairing is a best of three on different maps, and every match has a replay. Open tournaments (marked Open) are
        started on demand with any 2+ bots and don&apos;t count for the season.
      </PageHeader>

      {items == null ? (
        <Skeleton className="mt-8 h-64 rounded-[14px]" />
      ) : (
        <>
          <section className="mt-8">
            <SectionTitle>Upcoming and live</SectionTitle>
            <TournamentList items={upcoming} empty="Nothing scheduled yet." />
          </section>
          <section className="mt-10">
            <SectionTitle>Past tournaments</SectionTitle>
            <TournamentList items={past} empty="No tournament has finished yet." />
          </section>
        </>
      )}
    </div>
  )
}
