'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { ExternalLink } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { AgentRunCard } from '@/components/tanks/agent-run-card'
import { BotNameForm } from '@/components/tanks/bot-name-form'
import { MyMatches } from '@/components/tanks/my-matches'
import { UploadVersion } from '@/components/tanks/upload-version'
import { VersionList } from '@/components/tanks/version-list'
import { Skeleton } from '@/components/ui/skeleton'
import { api, friendlyMessage } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { MyTanks } from '@/lib/types'

const OPEN_PROOF_STATUSES = ['queued', 'claimed', 'running_agent', 'diff_submitted', 'running_sandbox']

// Whether anything on this page is still moving: a version's check hasn't
// resolved yet, or an agent run is open. Polling stops the moment neither is
// true, so a finished page sits still instead of hitting the API forever.
function needsPoll(data: MyTanks | null): boolean {
  if (!data) return false
  if (data.versions.some((v) => v.status === 'pending')) return true
  if (data.agent_runs.some((p) => OPEN_PROOF_STATUSES.includes(p.status))) return true
  return false
}

export default function TanksPage() {
  const { me } = useMe(5000)
  const [data, setData] = useState<MyTanks | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setData(await api<MyTanks>('/me/tanks'))
      setError(null)
    } catch (e) {
      setError(friendlyMessage(e))
    }
  }, [])

  const poll = needsPoll(data)
  useEffect(() => {
    void load()
    if (!poll) return
    const t = setInterval(() => { void load() }, 3000)
    return () => clearInterval(t)
  }, [load, poll])

  if (!me) return null

  return (
    <div className="space-y-10">
      <PageHeader kicker="Your bot on the ladder" title="Tanks">
        Write it by hand, upload an archive, or let your agent do it — every version goes through the same checks
        before it can join the ladder.
      </PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!data ? (
        <div className="space-y-6">
          <Skeleton className="h-36 rounded-[14px]" />
          <Skeleton className="h-52 rounded-[14px]" />
        </div>
      ) : (
        <div className="grid gap-8 lg:grid-cols-[1fr_20rem]">
          <div className="min-w-0 space-y-10">
            <section className="rounded-[14px] border border-border bg-card p-5">
              {data.bot ? (
                <>
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div className="min-w-0">
                      <h2 className="heading truncate text-xl">{data.bot.name}</h2>
                      <p className="mt-1 text-sm text-muted-foreground">
                        Active version: {data.bot.active_version != null ? `v${data.bot.active_version}` : 'none yet'}
                      </p>
                    </div>
                    <Link href={`/tanks/bots/${data.bot.id}`}
                      className="inline-flex shrink-0 items-center gap-1 text-sm font-semibold text-primary hover:underline">
                      Public profile<ExternalLink className="size-3.5" />
                    </Link>
                  </div>
                  <div className="mt-4 grid grid-cols-3 gap-3 sm:max-w-sm">
                    <div><p className="text-xs font-bold text-muted-foreground">Rating</p><p className="font-mono text-lg font-bold">{data.bot.rating}</p></div>
                    <div><p className="text-xs font-bold text-muted-foreground">Matches</p><p className="font-mono text-lg font-bold">{data.bot.matches}</p></div>
                    <div><p className="text-xs font-bold text-muted-foreground">Wins</p><p className="font-mono text-lg font-bold">{data.bot.wins}</p></div>
                  </div>
                  <div className="mt-5 border-t border-border pt-4">
                    <BotNameForm currentName={data.bot.name} onSaved={() => void load()} />
                  </div>
                </>
              ) : (
                <>
                  <h2 className="heading text-xl">No bot yet</h2>
                  <p className="mt-2 text-sm text-muted-foreground">
                    Upload a version or let your agent write one below, and a bot is created for you automatically.
                    Pick a name now, or rename it later.
                  </p>
                  <div className="mt-4">
                    <BotNameForm suggestedName={me.agent?.name} onSaved={() => void load()} />
                  </div>
                </>
              )}
            </section>

            <section>
              <SectionTitle aside={data.versions.length > 0 ? `${data.versions.length} total` : undefined}>Versions</SectionTitle>
              <VersionList versions={data.versions} />
            </section>

            {data.bot && (
              <section>
                <SectionTitle>My recent matches</SectionTitle>
                <MyMatches matches={data.matches} botId={data.bot.id} />
              </section>
            )}
          </div>

          <aside className="space-y-8">
            <AgentRunCard agent={me.agent} runs={data.agent_runs} onStarted={() => void load()} />
            <section className="rounded-[14px] border border-border bg-card p-5">
              <h2 className="heading">Upload by hand</h2>
              <div className="mt-3">
                <UploadVersion onUploaded={() => void load()} />
              </div>
            </section>
          </aside>
        </div>
      )}
    </div>
  )
}
