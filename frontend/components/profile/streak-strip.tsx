'use client'

import { Check } from 'lucide-react'
import { useT } from '@/lib/i18n/client'
import { formatDate } from '@/lib/i18n/core'
import { retentionMessages } from '@/lib/i18n/messages/retention'

// Monday of the current UTC week plus `i` days.
const dayStr = (i: number) => {
  const n = new Date()
  const sinceMonday = (n.getUTCDay() + 6) % 7
  return new Date(Date.UTC(n.getUTCFullYear(), n.getUTCMonth(), n.getUTCDate() - sinceMonday + i)).toISOString().slice(0, 10)
}

// The current UTC week, Monday first: solved, missed, today (pending until solved) and days still to come. `solved` is the list of days with a passing result.
export function StreakStrip({ solved, streak, hideNumbers }: { solved: string[]; streak: { current: number; best: number }; hideNumbers?: boolean }) {
  const t = useT(retentionMessages)
  const set = new Set(solved)
  const days = Array.from({ length: 7 }, (_, i) => dayStr(i))
  const todayStr = dayStr((new Date().getUTCDay() + 6) % 7)
  return (
    <div className="space-y-2">
      {!hideNumbers && (
        <p className="flex flex-wrap items-baseline gap-x-2 text-sm">
          <span className="font-bold">🔥 {t('streakNow', { n: streak.current })}</span>
          <span className="text-xs text-muted-foreground">{t('streakBest', { n: streak.best })}</span>
        </p>
      )}
      <ol className="flex gap-1.5" title={t('streakHelp')}>
        {days.map((d, i) => {
          const today = d === todayStr
          const future = d > todayStr
          const ok = set.has(d)
          const label = ok ? t('streakSolved') : today ? t('streakToday') : future ? '' : t('streakMissed')
          return (
            <li key={d} className="flex min-w-0 flex-1 flex-col items-center gap-1" title={`${formatDate(t.locale, d)}: ${ok && today ? t('streakTodayDone') : label}`}>
              <span aria-label={label}
                className={`flex size-7 items-center justify-center rounded-full text-xs font-bold sm:size-8 ${ok ? 'bg-success text-success-foreground' : future ? 'border border-dashed border-strong text-muted-foreground' : today ? 'border-2 border-dashed border-primary text-primary' : 'bg-muted text-muted-foreground'}`}>
                {ok ? <Check className="size-4" /> : today ? '•' : future ? '' : '×'}
              </span>
              <span className={`text-[10px] uppercase ${today ? 'font-bold text-foreground' : 'text-muted-foreground'}`}>{formatDate(t.locale, d, { weekday: 'short' })}</span>
            </li>
          )
        })}
      </ol>
    </div>
  )
}
