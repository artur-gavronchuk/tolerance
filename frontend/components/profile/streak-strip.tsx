'use client'

import { Check } from 'lucide-react'
import { useT } from '@/lib/i18n/client'
import { formatDate } from '@/lib/i18n/core'
import { retentionMessages } from '@/lib/i18n/messages/retention'

const dayStr = (offset: number) => {
  const n = new Date()
  return new Date(Date.UTC(n.getUTCFullYear(), n.getUTCMonth(), n.getUTCDate() - offset)).toISOString().slice(0, 10)
}

// The last seven UTC days: solved, missed, and today (pending until solved). `solved` is the list of days with a passing result.
export function StreakStrip({ solved, streak, hideNumbers }: { solved: string[]; streak: { current: number; best: number }; hideNumbers?: boolean }) {
  const t = useT(retentionMessages)
  const set = new Set(solved)
  const days = Array.from({ length: 7 }, (_, i) => dayStr(6 - i))
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
          const today = i === 6
          const ok = set.has(d)
          const label = ok ? t('streakSolved') : today ? t('streakToday') : t('streakMissed')
          return (
            <li key={d} className="flex min-w-0 flex-1 flex-col items-center gap-1" title={`${formatDate(t.locale, d)}: ${ok && today ? t('streakTodayDone') : label}`}>
              <span aria-label={label}
                className={`flex size-7 items-center justify-center rounded-full text-xs font-bold sm:size-8 ${ok ? 'bg-success text-success-foreground' : today ? 'border-2 border-dashed border-primary text-primary' : 'bg-muted text-muted-foreground'}`}>
                {ok ? <Check className="size-4" /> : today ? '•' : '×'}
              </span>
              <span className={`text-[10px] uppercase ${today ? 'font-bold text-foreground' : 'text-muted-foreground'}`}>{formatDate(t.locale, d, { weekday: 'short' })}</span>
            </li>
          )
        })}
      </ol>
    </div>
  )
}
