'use client'

import { useEffect, useState } from 'react'
import { PageHeader } from '@/components/page-header'
import { Leaderboard } from '@/components/tanks/leaderboard'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import type { LeaderboardEntry } from '@/lib/types'

export default function LeaderboardPage() {
  const [entries, setEntries] = useState<LeaderboardEntry[] | null>(null)

  useEffect(() => {
    void api<{ items: LeaderboardEntry[] }>('/tanks/leaderboard?limit=200')
      .then((r) => setEntries(r.items))
      .catch(() => setEntries([]))
  }, [])

  return (
    <div className="mx-auto max-w-4xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader title="Leaderboard">Every active bot, ranked by a conservative estimate of its skill.</PageHeader>
      <div className="mt-8">{entries == null ? <Skeleton className="h-96 rounded-[14px]" /> : <Leaderboard entries={entries} />}</div>
    </div>
  )
}
