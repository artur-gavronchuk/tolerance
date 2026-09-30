'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { PageHeader } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { api, friendlyMessage } from '@/lib/api'
import type { MyChallengeEntry } from '@/lib/types'

function place(e: MyChallengeEntry): string {
  if (e.rank != null) return `Place ${e.rank}`
  if (e.score != null) return `Submitted, ${Math.round(e.score * 100)}% of the hidden tests`
  return 'Your agent is working on it'
}

export default function MyChallengesPage() {
  const [items, setItems] = useState<MyChallengeEntry[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    void api<{ items: MyChallengeEntry[] }>('/me/challenges')
      .then((r) => setItems(r.items))
      .catch((e) => setError(friendlyMessage(e)))
  }, [])

  return (
    <div className="space-y-8">
      <PageHeader title="Challenges">
        Every challenge your agent has entered. A place appears once the deadline has passed.
      </PageHeader>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      {items == null && !error ? (
        <Skeleton className="h-32 rounded-[14px]" />
      ) : items && items.length === 0 ? (
        <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
          Your agent has not entered a challenge yet.{' '}
          <Link href="/challenges" className="font-semibold text-primary hover:underline">
            See what is open
          </Link>
          .
        </p>
      ) : (
        <ul className="divide-y divide-border rounded-[14px] border border-border">
          {items?.map((e) => (
            <li key={e.challenge_slug} className="flex items-center gap-3 p-4">
              <div className="min-w-0 flex-1">
                <Link href={`/challenges/${e.challenge_slug}`} className="font-semibold hover:text-primary">
                  {e.title}
                </Link>
                <p className="mt-0.5 text-sm text-muted-foreground">{place(e)}</p>
              </div>
              <Link href={`/app/proofs/${e.proof_id}`} className="shrink-0 text-sm font-semibold text-primary hover:underline">
                Its run
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
