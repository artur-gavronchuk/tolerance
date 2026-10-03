'use client'

import Link from 'next/link'
import { ArrowRight, History } from 'lucide-react'
import { StreakStrip } from '@/components/profile/streak-strip'
import { fmtScore } from '@/components/daily/daily-board'
import { useT } from '@/lib/i18n/client'
import { retentionMessages } from '@/lib/i18n/messages/retention'
import type { Recap, RecapResult } from '@/lib/types'

// Yesterday's result and the week's streak, with a nudge while today is unsolved.
export function RecapCard({ recap }: { recap: Recap }) {
  const t = useT(retentionMessages)
  const y = recap.yesterday
  const solvedToday = recap.solved_days.includes(recap.today)
  const optimize = y?.task.kind === 'optimize'
  const res = (r: RecapResult) => (optimize ? fmtScore(r.score, t.locale) : `${r.passed_tests}/${r.total_tests}`)
  return (
    <div className="mb-2 grid gap-3 rounded-xl border border-border bg-card p-3.5 sm:grid-cols-[1fr_auto] sm:items-center sm:gap-6">
      {y ? (
        <div className="min-w-0 space-y-1 text-sm">
          <h3 className="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
            <History className="size-3.5 text-primary" /><span className="truncate">{t('yesterdayTask', { title: y.task.title })}</span>
          </h3>
          {y.mine ? (
            <p className="font-bold">
              {optimize ? t('youScore', { score: res(y.mine) }) : t('youTests', { passed: y.mine.passed_tests, total: y.mine.total_tests })}
              <span className="font-mono"> · {y.place ? t('placeOf', { place: y.place, of: y.participants }) : t('notRanked')}</span>
            </p>
          ) : (
            <p className="font-bold">{t('youSkipped')}</p>
          )}
          <p className="truncate text-xs text-muted-foreground">
            {y.winner ? `${t('winner')}: ${t('winnerLine', { handle: y.winner.handle ?? '', result: res(y.winner) })}` : t('nobodyRanked')}
          </p>
          <Link href={`/day/${y.day}`} className="inline-flex items-center gap-1 text-xs font-semibold text-primary hover:underline">
            {t('seeDay')}<ArrowRight className="size-3" />
          </Link>
        </div>
      ) : <div />}
      <div className="space-y-2 sm:w-64">
        <StreakStrip solved={recap.solved_days} streak={recap.streak} />
        {!solvedToday && (
          <p className="text-xs font-semibold text-primary">
            {recap.streak.current > 0 ? t('nudgeKeep', { n: recap.streak.current }) : t('nudgeStart')}
          </p>
        )}
      </div>
    </div>
  )
}
