'use client'

import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import type { Submission } from '@/lib/types'
import { useT } from '@/lib/i18n/client'
import { dailyMessages } from '@/lib/i18n/messages/daily'

// The day's result as Wordle-style text: one row of squares per finished attempt, one square per hidden test.
export function shareText({ day, title, subs, streak, scoreWord = 'score', house }: { day: string; title: string; subs: Submission[]; streak: number; scoreWord?: string; house?: string | null }) {
  const rows = subs
    .filter((s) => s.status === 'passed' || s.status === 'failed')
    .sort((a, b) => a.created_at.localeCompare(b.created_at))
    .map((s) => {
      const squares = s.tests.length > 0
        ? s.tests.map((t) => (t.passed ? '🟩' : '🟥')).join('')
        : '⬛'.repeat(Math.max(1, s.total_tests))
      if (s.score != null) return `${s.tests.length > 20 ? '' : squares + ' '}${scoreWord} ${Math.round(s.score * 100) / 100}`
      return `${squares} ${s.passed_tests}/${s.total_tests}`
    })
  if (rows.length === 0) return null
  const made = subs.find((s) => s.status === 'passed')?.made_with || subs.find((s) => s.made_with)?.made_with
  const footer = [streak > 0 ? `🔥 ${streak}` : '', made ?? ''].filter(Boolean).join(' · ')
  const url = typeof window === 'undefined' ? 'https://tolerance.cc' : window.location.origin
  return [`tolerance · ${day}`, title, ...rows, house, footer, url].filter(Boolean).join('\n')
}

export function ShareResult({ text }: { text: string }) {
  const t = useT(dailyMessages)
  const [copied, setCopied] = useState(false)
  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {}
  }
  return (
    <div className="flex flex-col gap-3 rounded-[14px] border border-border bg-card p-4 sm:flex-row sm:items-start">
      <pre className="min-w-0 flex-1 font-mono text-sm leading-6 whitespace-pre-wrap [overflow-wrap:anywhere]">{text}</pre>
      <div className="flex shrink-0 gap-2">
        <Button size="sm" onClick={copy}>{copied ? <Check /> : <Copy />}{copied ? t('copied') : t('copyResult')}</Button>
        <Button size="sm" variant="outline" nativeButton={false}
          render={<a href={`https://x.com/intent/post?text=${encodeURIComponent(text)}`} target="_blank" rel="noreferrer" />}>
          {t('postOnX')}
        </Button>
      </div>
    </div>
  )
}
