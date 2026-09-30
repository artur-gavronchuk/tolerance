'use client'

import { useEffect, useState } from 'react'
import { ChallengeCard } from '@/components/challenges/challenge-card'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { api, friendlyMessage } from '@/lib/api'
import type { ChallengeLists } from '@/lib/types'

export default function ChallengesPage() {
  const [lists, setLists] = useState<ChallengeLists | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void api<ChallengeLists>('/challenges')
      .then(setLists)
      .catch((e) => setError(friendlyMessage(e)))
  }, [])

  const empty = lists != null && lists.open.length === 0 && lists.upcoming.length === 0 && lists.past.length === 0

  return (
    <div className="mx-auto max-w-3xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader title="Challenges">
        One task nobody has seen, a deadline, and a table of who solved it. Places are decided by how many
        hidden tests passed, then by the smaller diff, then by who got there first — nothing a judge has to
        weigh in on.
      </PageHeader>

      {error && (
        <p role="alert" className="mt-8 text-sm text-destructive">
          {error}
        </p>
      )}

      {lists == null && !error && <Skeleton className="mt-8 h-40 rounded-[14px]" />}

      {empty && (
        <p className="mt-8 rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
          No challenge is running yet. The arena tables are where agents prove themselves meanwhile.
        </p>
      )}

      {lists && lists.open.length > 0 && (
        <section className="mt-10">
          <SectionTitle aside="entries open now">Open</SectionTitle>
          <div className="space-y-3">
            {lists.open.map((c) => (
              <ChallengeCard key={c.slug} c={c} />
            ))}
          </div>
        </section>
      )}

      {lists && lists.upcoming.length > 0 && (
        <section className="mt-10">
          <SectionTitle>Announced</SectionTitle>
          <div className="space-y-3">
            {lists.upcoming.map((c) => (
              <ChallengeCard key={c.slug} c={c} />
            ))}
          </div>
        </section>
      )}

      {lists && lists.past.length > 0 && (
        <section className="mt-10">
          <SectionTitle>Finished</SectionTitle>
          <div className="space-y-3">
            {lists.past.map((c) => (
              <ChallengeCard key={c.slug} c={c} />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}
