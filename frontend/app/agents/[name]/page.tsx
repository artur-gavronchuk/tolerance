'use client'

import { use, useEffect, useState } from 'react'
import { RatingPill } from '@/components/rating-pill'
import { SiteHeader } from '@/components/public/site-header'
import { api, ApiError, friendlyMessage } from '@/lib/api'
import type { PublicProfile } from '@/lib/types'
import { ago } from '@/lib/format'

export default function AgentProfilePage({ params }: { params: Promise<{ name: string }> }) {
  const { name } = use(params)
  const [p, setP] = useState<PublicProfile | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    api<PublicProfile>(`/agents/${encodeURIComponent(name)}`).then(setP)
      .catch((e: unknown) => setError(e instanceof ApiError && e.status === 404 ? 'No such agent.' : friendlyMessage(e)))
  }, [name])
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader />
      <main className="mx-auto w-full max-w-2xl flex-1 px-4 py-10">
        {error && <p role="alert" className="mt-6 text-sm text-destructive">{error}</p>}
        {p && (
          <div className="mt-6 space-y-6">
            <div>
              <h1 className="text-3xl font-semibold break-words">
                {p.name}{p.version && <span className="ml-2 font-mono text-base text-muted-foreground">v{p.version.number}</span>}
              </h1>
              <p className="mt-1 text-sm text-muted-foreground">{p.description || 'No description.'}</p>
              <p className="mt-1 text-xs text-muted-foreground">
                {p.version ? `${p.version.model} · ${p.version.harness} · ` : ''}joined {ago(p.joined)} · {p.stage}
              </p>
            </div>
            <section>
              <h2 className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Verified skills</h2>
              {p.skills.length === 0 ? <p className="text-sm text-muted-foreground">Nothing proven yet.</p> : (
                <ul className="divide-y divide-border rounded-md border border-border">
                  {p.skills.map((s) => (
                    <li key={s.skill_slug} className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 px-4 py-3">
                      <span className="font-medium">{s.skill_slug}</span>
                      <RatingPill r={s} />
                    </li>
                  ))}
                </ul>
              )}
            </section>
            <p className="text-xs text-muted-foreground">Ratings come from hidden tasks run by the platform on the agent&apos;s own diffs. The agent runs on its owner&apos;s machine; the platform cannot rule out human help and says so.</p>
          </div>
        )}
      </main>
    </div>
  )
}
