'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { Download } from 'lucide-react'
import { track } from '@/lib/analytics'
import { Markdown } from '@/components/daily/markdown'
import { Countdown } from '@/components/daily/countdown'
import { DailyBoard, fmtScore } from '@/components/daily/daily-board'
import { DayRevealView } from '@/components/daily/day-reveal'
import { DayStatsPanel } from '@/components/daily/day-stats'
import { HouseStrip, houseShareLine } from '@/components/daily/house-strip'
import { ShareResult, shareText } from '@/components/daily/share-result'
import { SubmissionCard } from '@/components/daily/submission-card'
import { TaskBadges } from '@/components/daily/task-header'
import { UploadForm } from '@/components/daily/upload-form'
import { AgentUpload } from '@/components/upload-link/agent-upload'
import { CopyBlock } from '@/components/copy-block'
import { SectionTitle } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { api, ApiError } from '@/lib/api'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { dailyMessages } from '@/lib/i18n/messages/daily'
import { useMe } from '@/lib/use-me'
import type { Daily, DailyRow, DayStats, Submission } from '@/lib/types'

export const OPTIMIZE_PROMPT = (direction: 'max' | 'min' | null) => `Here is a repository and a task description in TASK.md. Improve solve.py to ${direction === 'min' ? 'minimize' : 'maximize'} the score described in TASK.md.
Test locally with tools/score.py on the instances in examples/. Keep the output valid and within the time limit per case. Use only the standard library.`

export const AGENT_PROMPT = `Here is a repository and a task description in TASK.md. Read TASK.md and fix the issue it describes.
Do not modify or delete existing tests, and do not add test files. Keep the change minimal and in the style of the surrounding code.
When you are done, the repository should build and the tests should pass.`

// The task of the day (`day` omitted) or a past day (`day` = YYYY-MM-DD).
// Past days accept practice uploads: they do not count for the leaderboards.
export function DailyView({ day }: { day?: string }) {
  const t = useT(dailyMessages)
  const { me, loading: meLoading, refresh: refreshMe } = useMe()
  const [daily, setDaily] = useState<Daily | null>(null)
  const [rows, setRows] = useState<DailyRow[] | null>(null)
  const [stats, setStats] = useState<DayStats | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [missing, setMissing] = useState(false)
  // Submissions made or polled in this session, kept apart from the server's list.
  const [local, setLocal] = useState<Submission[]>([])

  const load = useCallback(async () => {
    try {
      const d = await api<Daily>(day ? `/daily/${day}` : '/daily')
      setDaily(d)
      setError(null)
      const [lb, st] = await Promise.all([
        api<{ items: DailyRow[] }>(`/daily/${d.day}/leaderboard`),
        api<DayStats>(`/daily/${d.day}/stats`),
      ])
      setRows(lb.items)
      setStats(st)
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) setMissing(true)
      else setError(errorText(e, t.locale))
    }
  }, [day])
  useEffect(() => { void load() }, [load])

  const update = useCallback((s: Submission) => {
    setLocal((cur) => {
      const prev = cur.find((x) => x.id === s.id)
      if (prev && ['queued', 'running'].includes(prev.status) && !['queued', 'running'].includes(s.status)) {
        void load()
        void refreshMe()
      }
      return prev ? cur.map((x) => (x.id === s.id ? s : x)) : [s, ...cur]
    })
  }, [load, refreshMe])

  if (missing) notFound()
  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!daily) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-12 w-72 max-w-full" />
        <Skeleton className="h-64 rounded-xl" />
      </div>
    )
  }

  const { task } = daily
  const practice = !daily.is_open
  const signedIn = !!me && !!daily.my
  const signedOut = !meLoading && !me
  const loginHref = `/login?next=${encodeURIComponent(day ? `/day/${day}` : '/')}`
  const attemptsLeft = practice || !daily.my ? undefined : Math.max(0, daily.attempts_per_day - daily.my.attempts_used)
  const serverIds = new Set(daily.my?.submissions.map((s) => s.id))
  const localById = new Map(local.map((s) => [s.id, s]))
  const subs = [
    ...local.filter((s) => !serverIds.has(s.id)),
    ...(daily.my?.submissions ?? []).map((s) => localById.get(s.id) ?? s),
  ]
  const share = shareText({ day: daily.day, title: task.title, subs, streak: daily.is_open ? me?.streak.current ?? 0 : 0, scoreWord: t('shareScore'), house: houseShareLine(daily.house ?? [], t) })

  return (
    <div className="space-y-10">
      <header className="flex flex-col gap-3">
        <p className="text-sm font-semibold text-muted-foreground">
          {practice ? <>{t('kickerArchive', { day: daily.day })} <Link href="/days" className="text-primary hover:underline">{t('allDays')}</Link></> : t('kickerToday', { day: daily.day })}
        </p>
        <h1 className="display text-title-sm break-words sm:text-title">{task.title}</h1>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <TaskBadges language={task.language} difficulty={task.difficulty} kind={task.kind} direction={task.direction} />
          {daily.is_open ? (
            <span className="text-sm text-muted-foreground">{t('closesIn')} <Countdown closesAt={daily.closes_at} onClosed={() => void load()} /></span>
          ) : (
            <span className="text-sm text-muted-foreground">{t('closedPractice')}</span>
          )}
        </div>
      </header>

      <div className="grid items-start gap-8 lg:grid-cols-[1fr_22rem]">
        <section className="min-w-0 rounded-xl border border-border bg-card p-5 sm:p-6">
          <Markdown>{task.task_md}</Markdown>
        </section>

        <aside className="order-first min-w-0 space-y-6 lg:sticky lg:top-6 lg:order-none lg:max-h-[calc(100dvh-3rem)] lg:overflow-y-auto">
          <section className="rounded-xl border border-border bg-card p-5">
            <h2 className="heading">{t('howTitle')}</h2>
            <ol className="mt-3 list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground">
              <li>{t('how1')}</li>
              <li>{t('how2')}</li>
            </ol>
            <div className="mt-3"><CopyBlock text={task.kind === 'optimize' ? OPTIMIZE_PROMPT(task.direction) : AGENT_PROMPT} /></div>
            <ol start={3} className="mt-3 list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground">
              <li>{t('how3')}</li>
            </ol>
            <div className="mt-3"><CopyBlock text={`cd ${task.slug} && zip -r ../solution.zip . -x '.git/*'`} /></div>
            {!practice && <p className="mt-3 text-xs text-muted-foreground">{t('revealNote')}</p>}
            <Button className="mt-4 w-full" render={<a href={task.repo_url} download onClick={() => track({ name: 'daily.download' })} />} nativeButton={false}>
              <Download />{t('downloadRepo')}
            </Button>
          </section>

          <section id="submit" className="scroll-mt-6 rounded-xl border border-border bg-card p-5">
            <h2 className="heading">{practice ? t('practiceTitle') : t('submitTitle')}</h2>
            {signedIn ? (
              <div className="mt-3 space-y-3">
                {practice && (
                  <p className="text-sm text-muted-foreground">{t('practiceOver')}</p>
                )}
                {!practice && <AgentUpload target={{ kind: 'daily' }} />}
                <UploadForm taskSlug={practice ? task.slug : undefined} attemptsLeft={attemptsLeft} onSubmitted={update} />
              </div>
            ) : signedOut ? (
              <div className="mt-3 space-y-3">
                <p className="text-sm text-muted-foreground">
                  {practice
                    ? t('signInPractice')
                    : t('signInSubmit')}
                </p>
                <Button className="w-full sm:w-auto" render={<Link href={loginHref} />} nativeButton={false}>{t('signInToSubmit')}</Button>
              </div>
            ) : (
              <Skeleton className="mt-3 h-32 rounded-lg" />
            )}
          </section>
        </aside>
      </div>

      {signedIn && daily.my && (
        <section>
          <SectionTitle aside={daily.my.best ? (task.kind === 'optimize' ? t('bestScore', { score: fmtScore(daily.my.best.score, t.locale) }) : t('best', { passed: daily.my.best.passed_tests, total: daily.my.best.total_tests })) : undefined}>
            {t('mySubs')}
          </SectionTitle>
          {share && <div className="mb-3"><ShareResult text={share} /></div>}
          {subs.length === 0 ? (
            <p className="rounded-xl border border-dashed border-strong px-5 py-8 text-center text-sm text-muted-foreground">{t('nothingYet')}</p>
          ) : (
            <ul className="divide-y divide-border rounded-xl border border-border bg-card">
              {subs.map((s) => <SubmissionCard key={s.id} sub={s} onUpdate={update} />)}
            </ul>
          )}
        </section>
      )}

      <section>
        <SectionTitle aside={practice ? t('finalStandings') : t('live')}>{t('leaderboard')}</SectionTitle>
        <div className="grid gap-6 lg:grid-cols-[1fr_22rem]">
          <div className="min-w-0">
            <HouseStrip house={daily.house ?? []} optimize={task.kind === 'optimize'} />
            {rows == null ? <Skeleton className="h-48 rounded-xl" /> : <DailyBoard rows={rows} me={me?.user.handle} optimize={task.kind === 'optimize'} />}
          </div>
          {stats && <DayStatsPanel stats={stats} />}
        </div>
      </section>

      {practice && <DayRevealView day={daily.day} />}
    </div>
  )
}
