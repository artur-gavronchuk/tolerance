import { Badge } from '@/components/ui/badge'
import { errorText } from '@/lib/i18n/messages/errors'
import { formatDate, formatDateTime, type Locale, type T } from '@/lib/i18n/core'
import { useT } from '@/lib/i18n/client'
import { productsMessages } from '@/lib/i18n/messages/products'
import type { ProductTask } from '@/lib/types'

export type PT = T<typeof productsMessages.en>
export const usePT = () => useT(productsMessages)

export function PhaseBadge({ phase }: { phase: ProductTask['phase'] }) {
  const t = usePT()
  return <Badge variant={phase === 'open' ? 'default' : phase === 'voting' ? 'outline' : 'secondary'}>{t(`phase.${phase}`)}</Badge>
}

function span(iso: string, now: number) {
  const m = Math.max(0, Math.round((new Date(iso).getTime() - now) / 60000))
  if (m >= 1440) return { key: 'dh', d: Math.floor(m / 1440), h: Math.floor((m % 1440) / 60), m: 0 } as const
  if (m >= 60) return { key: 'hm', d: 0, h: Math.floor(m / 60), m: m % 60 } as const
  return { key: 'm', d: 0, h: 0, m: Math.max(1, m) } as const
}

// "in 2d 5h" / "in 3h 10m" / "in 12m" until an instant.
export function until(t: PT, iso: string, now = Date.now()) {
  const s = span(iso, now)
  return t(`in.${s.key}`, { d: s.d, h: s.h, m: s.m })
}

// The same without the "in", for "2d 5h left".
export function untilBare(t: PT, iso: string, now = Date.now()) {
  const s = span(iso, now)
  return t(`bare.${s.key}`, { d: s.d, h: s.h, m: s.m })
}

// "5m ago" style age of a past instant.
export function ago(t: PT, iso: string, now = Date.now()) {
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (s < 60) return t('ago.s', { n: s })
  if (s < 3600) return t('ago.m', { n: Math.round(s / 60) })
  if (s < 86400) return t('ago.h', { n: Math.round(s / 3600) })
  return t('ago.d', { n: Math.round(s / 86400) })
}

// Every time on the product pages is shown in UTC, with the suffix, so nobody has to guess the zone.
export const utc = (locale: Locale, iso: string) => formatDateTime(locale, iso, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false })
export const utcDay = (locale: Locale, iso: string) => formatDate(locale, iso)

// Server messages for known codes, translated; unknown ones fall back to the server's text.
export const friendly = (t: PT, err: unknown): string => errorText(err, t.locale)

// True while a site task's entries are judged blind: automated check counts and source stay hidden so they
// can't sway the votes.
export const isBlind = (task: Pick<ProductTask, 'kind' | 'phase'>) => task.kind === 'site' && task.phase === 'voting'

// One line on what the ranking is, for page headers.
export function rankSummary(t: PT, kind: ProductTask['kind']) {
  return kind === 'site' ? t('rank.siteSummary') : t('rank.cliSummary')
}

// How a task's standings are decided, in words (one rule, stated once); shown inside the disclosure.
export function rankRuleText(t: PT, kind: ProductTask['kind'], hasChecks: boolean) {
  if (kind === 'site') return t('rank.siteRule') + (hasChecks ? t('rank.siteRuleChecks') : '') + t('rank.siteRuleEnd') + t('rank.qualityNote')
  return t('rank.cliRule')
}

// The collapsed method behind a ranking.
export function RankingHow({ kind, hasChecks }: { kind: ProductTask['kind']; hasChecks: boolean }) {
  const t = usePT()
  return (
    <details className="max-w-2xl rounded-[10px] border border-border bg-card px-4 py-2.5 text-sm">
      <summary className="cursor-pointer font-semibold">{t('rank.how')}</summary>
      <p className="mt-2 text-muted-foreground">{rankRuleText(t, kind, hasChecks)}</p>
    </details>
  )
}

// Puts `<b>` around the text that replaces the \0 marker.
function bold(s: string, text: string) {
  const [pre, post] = s.split('\u0000')
  return <>{pre}<b className="text-foreground">{text}</b>{post}</>
}

// The one-line state of the voting window. With `judge` it also says how site entries are judged (the compare
// panel says that itself on the task page).
export function VotingNote({ task, judge = false }: { task: ProductTask; judge?: boolean }) {
  const t = usePT()
  if (task.phase === 'open') return null
  const ends = utc(t.locale, task.voting_ends_at)
  return (
    <p className="rounded-[10px] border border-border bg-muted/50 px-4 py-2.5 text-sm text-muted-foreground">
      {task.phase === 'voting'
        ? <>{bold(t('note.votingOpen', { ends: '\u0000', left: until(t, task.voting_ends_at) }), ends)}{task.kind === 'cli' ? t('note.voteTool') : judge ? ` ${t('judge.sentence')}` : ''}</>
        : <>{t('note.ended', { ends })}</>}
    </p>
  )
}
