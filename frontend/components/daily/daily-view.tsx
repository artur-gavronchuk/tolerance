'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { Download } from 'lucide-react'
import { Markdown } from '@/components/daily/markdown'
import { Countdown } from '@/components/daily/countdown'
import { DailyBoard, fmtScore } from '@/components/daily/daily-board'
import { DayRevealView } from '@/components/daily/day-reveal'
import { DayStatsPanel } from '@/components/daily/day-stats'
import { ShareResult, shareText } from '@/components/daily/share-result'
import { SubmissionCard } from '@/components/daily/submission-card'
import { TaskBadges } from '@/components/daily/task-header'
import { UploadForm } from '@/components/daily/upload-form'
import { CopyBlock } from '@/components/copy-block'
import { SectionTitle } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { api, ApiError, friendlyMessage } from '@/lib/api'
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
  const { me, refresh: refreshMe } = useMe()
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
      else setError(friendlyMessage(e))
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
        <Skeleton className="h-64 rounded-[14px]" />
      </div>
    )
  }

  const { task } = daily
  const practice = !daily.is_open
  const attemptsLeft = practice || !daily.my ? undefined : Math.max(0, daily.attempts_per_day - daily.my.attempts_used)
  const serverIds = new Set(daily.my?.submissions.map((s) => s.id))
  const localById = new Map(local.map((s) => [s.id, s]))
  const subs = [
    ...local.filter((s) => !serverIds.has(s.id)),
    ...(daily.my?.submissions ?? []).map((s) => localById.get(s.id) ?? s),
  ]
  const share = shareText({ day: daily.day, title: task.title, subs, streak: daily.is_open ? me?.streak.current ?? 0 : 0 })

  return (
    <div className="space-y-10">
      <header className="flex flex-col gap-3">
        <p className="text-sm font-semibold text-muted-foreground">
          {practice ? <>Archive · {daily.day} · <Link href="/days" className="text-primary hover:underline">all days</Link></> : <>Task of the day · {daily.day}</>}
        </p>
        <h1 className="display text-[2.1rem] break-words sm:text-[2.75rem]">{task.title}</h1>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <TaskBadges language={task.language} difficulty={task.difficulty} kind={task.kind} direction={task.direction} />
          {daily.is_open ? (
            <span className="text-sm text-muted-foreground">Closes in <Countdown closesAt={daily.closes_at} onClosed={() => void load()} /></span>
          ) : (
            <span className="text-sm text-muted-foreground">Closed. Uploads here are practice and do not count.</span>
          )}
        </div>
      </header>

      <div className="grid gap-8 lg:grid-cols-[1fr_22rem]">
        <section className="min-w-0 rounded-[14px] border border-border bg-card p-5 sm:p-6">
          <Markdown>{task.task_md}</Markdown>
        </section>

        <aside className="min-w-0 space-y-6">
          <section className="rounded-[14px] border border-border bg-card p-5">
            <h2 className="heading">How to solve</h2>
            <ol className="mt-3 list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground">
              <li>Download the repository and unpack it.</li>
              <li>Give it to your coding agent (Claude Code, Cursor, Codex, anything) with this prompt:</li>
            </ol>
            <div className="mt-3"><CopyBlock text={task.kind === 'optimize' ? OPTIMIZE_PROMPT(task.direction) : AGENT_PROMPT} /></div>
            <ol start={3} className="mt-3 list-decimal space-y-1.5 pl-5 text-sm text-muted-foreground">
              <li>Zip the edited repository, or save your change as a .patch, and upload it below.</li>
            </ol>
            <div className="mt-3"><CopyBlock text={`cd ${task.slug} && zip -r ../solution.zip . -x '.git/*'`} /></div>
            {!practice && <p className="mt-3 text-xs text-muted-foreground">When the day closes, the hidden tests and the first passing solutions are published.</p>}
            <Button className="mt-4 w-full" render={<a href={task.repo_url} download />} nativeButton={false}>
              <Download />Download repo
            </Button>
          </section>

          <section className="rounded-[14px] border border-border bg-card p-5">
            <h2 className="heading">{practice ? 'Practice upload' : 'Submit'}</h2>
            {!daily.my ? (
              <div className="mt-3 space-y-3">
                <p className="text-sm text-muted-foreground">Sign in to upload your result and get on the leaderboard.</p>
                <Button render={<Link href="/login" />} nativeButton={false}>Sign in to submit</Button>
              </div>
            ) : (
              <div className="mt-3 space-y-3">
                {practice && (
                  <p className="text-sm text-muted-foreground">This day is over: you can still try it, but the result will not count for any leaderboard or streak.</p>
                )}
                <UploadForm taskSlug={practice ? task.slug : undefined} attemptsLeft={attemptsLeft} onSubmitted={update} />
              </div>
            )}
          </section>
        </aside>
      </div>

      {daily.my && (
        <section>
          <SectionTitle aside={daily.my.best ? (task.kind === 'optimize' ? `Best score: ${fmtScore(daily.my.best.score)}` : `Best: ${daily.my.best.passed_tests}/${daily.my.best.total_tests}`) : undefined}>
            My submissions
          </SectionTitle>
          {share && <div className="mb-3"><ShareResult text={share} /></div>}
          {subs.length === 0 ? (
            <p className="rounded-[14px] border border-dashed border-input px-5 py-8 text-center text-sm text-muted-foreground">Nothing submitted yet.</p>
          ) : (
            <ul className="divide-y divide-border rounded-[14px] border border-border bg-card">
              {subs.map((s) => <SubmissionCard key={s.id} sub={s} onUpdate={update} />)}
            </ul>
          )}
        </section>
      )}

      <section>
        <SectionTitle aside={practice ? 'Final standings' : 'Live'}>Leaderboard</SectionTitle>
        <div className="grid gap-6 lg:grid-cols-[1fr_22rem]">
          <div className="min-w-0">
            {rows == null ? <Skeleton className="h-48 rounded-[14px]" /> : <DailyBoard rows={rows} me={me?.user.handle} optimize={task.kind === 'optimize'} />}
          </div>
          {stats && <DayStatsPanel stats={stats} />}
        </div>
      </section>

      {practice && <DayRevealView day={daily.day} />}
    </div>
  )
}
