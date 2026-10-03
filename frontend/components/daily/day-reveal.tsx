'use client'

import { HideButton } from '@/components/admin/hide-button'
import { useEffect, useState } from 'react'
import { SectionTitle } from '@/components/page-header'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { StackName } from '@/components/daily/stack-label'
import type { DayReveal } from '@/lib/types'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { dailyMessages } from '@/lib/i18n/messages/daily'

const box = 'rounded-[14px] border border-border bg-card'
const pre = 'max-h-[32rem] overflow-auto border-t border-border p-3 font-mono text-xs leading-5'

// A closed day's hidden tests and the earliest fully passing solutions.
export function DayRevealView({ day }: { day: string }) {
  const t = useT(dailyMessages)
  const [data, setData] = useState<DayReveal | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    api<DayReveal>(`/daily/${day}/reveal`).then(setData).catch((e) => setError(errorText(e, t.locale)))
  }, [day])

  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!data) return <Skeleton className="h-32 rounded-[14px]" />

  return (
    <div className="space-y-10">
      <section>
        <SectionTitle aside={t('shown', { n: data.solutions.length })}>{t('solutions')}</SectionTitle>
        {data.solutions.length === 0 ? (
          <p className="rounded-[14px] border border-dashed border-strong px-5 py-8 text-center text-sm text-muted-foreground">{t('noSolutions')}</p>
        ) : (
          <ul className="space-y-3">
            {data.solutions.map((s) => (
              <li key={s.place}>
                <details className={box}>
                  <summary className="flex cursor-pointer flex-wrap items-baseline gap-x-3 gap-y-1 px-4 py-3">
                    <span className="font-mono text-sm text-muted-foreground">#{s.place}</span>
                    <span className="font-semibold">{s.handle}</span>
                    <span className="min-w-0 truncate text-sm text-muted-foreground"><StackName madeWith={s.made_with} /></span>
                    <span className="ml-auto font-mono text-xs text-muted-foreground">{diffStat(s.diff)} · {s.submitted_at.slice(11, 16)} UTC</span>
                  </summary>
                  <Diff text={s.diff} />
                </details>
                <HideButton kind="submission" id={s.id} />
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <SectionTitle aside={t.plural('files', data.hidden_tests.length)}>{t('hiddenTests')}</SectionTitle>
        <ul className="space-y-3">
          {data.hidden_tests.map((f) => (
            <li key={f.path}>
              <details className={box}>
                <summary className="cursor-pointer px-4 py-3 font-mono text-sm font-semibold break-all">{f.path}</summary>
                <pre className={pre}>{f.content}</pre>
              </details>
            </li>
          ))}
        </ul>
      </section>
    </div>
  )
}

function diffStat(diff: string) {
  let add = 0
  let del = 0
  for (const l of diff.split('\n')) {
    if (l.startsWith('+') && !l.startsWith('+++')) add++
    else if (l.startsWith('-') && !l.startsWith('---')) del++
  }
  return `+${add} −${del}`
}

function Diff({ text }: { text: string }) {
  return (
    <pre className={pre}>
      {text.split('\n').map((l, i) => (
        <div key={i} className={lineClass(l)}>{l || ' '}</div>
      ))}
    </pre>
  )
}

function lineClass(l: string) {
  if (l.startsWith('+++') || l.startsWith('---') || l.startsWith('diff ')) return 'font-bold'
  if (l.startsWith('+')) return 'bg-success/10 text-success'
  if (l.startsWith('-')) return 'bg-destructive/10 text-destructive'
  if (l.startsWith('@@')) return 'text-muted-foreground'
  return undefined
}
