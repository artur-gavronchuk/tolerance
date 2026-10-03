'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { ExternalLink } from 'lucide-react'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { MakeBot } from '@/components/tanks/make-bot'
import { BotNameForm } from '@/components/tanks/bot-name-form'
import { MyMatches } from '@/components/tanks/my-matches'
import { CopyReportButton } from '@/components/tanks/copy-report-button'
import { VersionList } from '@/components/tanks/version-list'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n/client'
import { tanksOwnerMessages as m, ownerError } from '@/lib/i18n/messages/tanks-owner'
import { useMe } from '@/lib/use-me'
import type { MyTanks } from '@/lib/types'

// While a version's check is pending the page refreshes every 3 s; afterwards every 10 s so new ladder matches
// show up without a reload. Polling only runs while the tab is visible.
const FAST_POLL_MS = 3000
const SLOW_POLL_MS = 10000

function pollInterval(data: MyTanks | null): number {
  return data && data.versions.some((v) => v.status === 'pending') ? FAST_POLL_MS : SLOW_POLL_MS
}

export default function TanksPage() {
  const { me } = useMe(5000)
  const t = useT(m)
  const [data, setData] = useState<MyTanks | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setData(await api<MyTanks>('/me/tanks'))
      setError(null)
    } catch (e) {
      setError(ownerError(t, e))
    }
  }, [t])

  const pollMs = pollInterval(data)
  useEffect(() => {
    void load()
    const t = setInterval(() => {
      if (document.visibilityState === 'visible') void load()
    }, pollMs)
    const onVisible = () => { if (document.visibilityState === 'visible') void load() }
    document.addEventListener('visibilitychange', onVisible)
    return () => {
      clearInterval(t)
      document.removeEventListener('visibilitychange', onVisible)
    }
  }, [load, pollMs])

  if (!me) return null

  return (
    <div className="space-y-10">
      <PageHeader kicker={t('page.kicker')} title={t('page.title')}>
        {t('page.intro')}
      </PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!data ? (
        <div className="space-y-6">
          <Skeleton className="h-36 rounded-[14px]" />
          <Skeleton className="h-52 rounded-[14px]" />
        </div>
      ) : (
        <div>
          <div className="min-w-0 space-y-10">
            {data.bot && (
              <section className="rounded-[14px] border border-border bg-card p-5">
                <>
                  <div className="flex flex-wrap items-start justify-between gap-4">
                    <div className="min-w-0">
                      <h2 className="heading truncate text-xl">{data.bot.name}</h2>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {t('page.activeVersion', { v: data.bot.active_version != null ? `v${data.bot.active_version}` : t('page.none') })}
                      </p>
                    </div>
                    <Link href={`/tanks/bots/${data.bot.id}`}
                      className="inline-flex shrink-0 items-center gap-1 text-sm font-semibold text-primary hover:underline">
                      {t('page.publicProfile')}<ExternalLink className="size-3.5" />
                    </Link>
                  </div>
                  <div className="mt-4 grid grid-cols-3 gap-3 sm:max-w-sm">
                    <div><p className="text-xs font-bold text-muted-foreground">{t('page.rating')}</p><p className="font-mono text-lg font-bold">{data.bot.rating}</p></div>
                    <div><p className="text-xs font-bold text-muted-foreground">{t('page.matches')}</p><p className="font-mono text-lg font-bold">{data.bot.matches}</p></div>
                    <div><p className="text-xs font-bold text-muted-foreground">{t('page.wins')}</p><p className="font-mono text-lg font-bold">{data.bot.wins}</p></div>
                  </div>
                  <div className="mt-5 border-t border-border pt-4">
                    <BotNameForm currentName={data.bot.name} onSaved={() => void load()} />
                  </div>
                </>
              </section>
            )}

            <MakeBot onUploaded={() => void load()} />

            {!data.bot && (
              <section className="rounded-[14px] border border-border bg-card p-5">
                <p className="mb-3 text-sm text-muted-foreground">
                  {t('page.firstUpload')}
                </p>
                <BotNameForm suggestedName={me.user.handle} onSaved={() => void load()} />
              </section>
            )}

            <section>
              <SectionTitle aside={data.versions.length > 0 ? t('page.total', { n: data.versions.length }) : undefined}>{t('page.versions')}</SectionTitle>
              <VersionList versions={data.versions} />
            </section>

            {data.bot && (
              <section>
                <SectionTitle>{t('page.recent')}</SectionTitle>
                {data.matches.length > 0 && (
                  <div className="mb-3 space-y-1">
                    <CopyReportButton matchIds={data.matches.slice(0, 5).map((m) => m.id)} label={t('page.copyLast5')}
                      variant="secondary" />
                    <p className="text-xs text-muted-foreground">
                      {t('page.reportsHelp')}
                    </p>
                  </div>
                )}
                <MyMatches matches={data.matches} botId={data.bot.id} />
              </section>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
