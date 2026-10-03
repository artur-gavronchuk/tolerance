'use client'

import { Bot, Check } from 'lucide-react'
import { fmtScore } from '@/components/daily/daily-board'
import { useT } from '@/lib/i18n/client'
import type { T } from '@/lib/i18n/core'
import { dailyMessages } from '@/lib/i18n/messages/daily'
import type { HouseResult } from '@/lib/types'

type DailyT = T<typeof dailyMessages.en>

// "Claude Code · Opus" -> "Opus": the part after the last dot is the model, which is what people compare with.
export function shortName(name: string) {
  const last = name.split('·').pop()?.trim()
  return last || name
}

function resultText(h: HouseResult, optimize: boolean, t: DailyT) {
  switch (h.state) {
    case 'done': return optimize ? fmtScore(h.score, t.locale) : `${h.passed_tests}/${h.total_tests}`
    case 'running': return t('houseRunning')
    case 'pending': return t('housePending')
    case 'error': return t('houseError')
    default: return t('houseNoResult')
  }
}

// The share card's line about the house agents the person beat or matched; null when neither.
export function houseShareLine(house: HouseResult[], t: DailyT) {
  const names = (r: 'beat' | 'tied') => house.filter((h) => h.vs_me?.result === r).map((h) => shortName(h.name)).join(', ')
  const beat = names('beat')
  if (beat) return t('shareBeat', { names: beat })
  const tied = names('tied')
  return tied ? t('shareTied', { names: tied }) : null
}

// "Beat the house": what each platform agent scored today, how many people beat the toughest, and where the
// signed-in person stands against each.
export function HouseStrip({ house, optimize }: { house: HouseResult[]; optimize: boolean }) {
  const t = useT(dailyMessages)
  if (house.length === 0) return null
  const done = house.filter((h) => h.state === 'done')
  // The toughest agent is the one fewest people have beaten (ties: the first listed).
  const toughest = done.reduce<HouseResult | null>((a, h) => (a == null || h.people_beat < a.people_beat ? h : a), null)
  const mine = house.filter((h) => h.vs_me)
  const gapText = (gap: number) => (optimize ? fmtScore(gap, t.locale) : t.plural('houseTests', Math.round(gap)))
  return (
    <div className="mb-3 space-y-3 rounded-[14px] border border-border bg-card p-4">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h3 className="flex items-center gap-1.5 text-sm font-semibold"><Bot className="size-4 text-muted-foreground" aria-hidden />{t('houseTitle')}</h3>
        <ul className="flex flex-wrap gap-2">
          {house.map((h) => (
            <li key={h.handle} title={h.name}
              className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm ${h.vs_me?.result === 'beat' ? 'border-primary/40 bg-accent' : 'border-border bg-background'}`}>
              <span className="font-semibold">{shortName(h.name)}</span>
              <span className={h.state === 'done' ? 'font-mono font-bold' : 'text-muted-foreground'}>{resultText(h, optimize, t)}</span>
              {h.vs_me?.result === 'beat' && <Check className="size-3.5 text-primary" aria-hidden />}
            </li>
          ))}
        </ul>
      </div>
      {toughest && (
        <p className="text-sm text-muted-foreground">
          {toughest.people_beat > 0
            ? t.plural('houseBeaten', toughest.people_beat, { name: shortName(toughest.name) })
            : t('houseNobody', { name: shortName(toughest.name) })}
        </p>
      )}
      {mine.length > 0 && (
        <ul className="flex flex-wrap gap-x-4 gap-y-1 text-sm font-semibold">
          {mine.map((h) => {
            const name = shortName(h.name)
            const v = h.vs_me!
            return (
              <li key={h.handle} className={v.result === 'behind' ? 'text-muted-foreground' : 'text-primary'}>
                {v.result === 'beat' ? t('vsBeat', { name }) : v.result === 'tied' ? t('vsTied', { name }) : t('vsBehind', { name, gap: gapText(v.gap) })}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
