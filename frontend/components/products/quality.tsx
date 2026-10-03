'use client'

import { Badge } from '@/components/ui/badge'
import type { A11yCounts, SiteQuality } from '@/lib/types'
import { usePT, type PT } from './phase'

const IMPACTS = ['critical', 'serious', 'moderate', 'minor'] as const

// The worse of the two widths per impact: what a visitor on either device meets.
function worst(q: SiteQuality): A11yCounts | null {
  const a = q.a11y
  if (!a) return null
  return Object.fromEntries(IMPACTS.map((k) => [k, Math.max(a.desktop[k], a.mobile[k])])) as unknown as A11yCounts
}

const sum = (c: A11yCounts) => IMPACTS.reduce((n, k) => n + c[k], 0)

function kb(t: PT, bytes: number) {
  const n = new Intl.NumberFormat(t.locale, { maximumFractionDigits: bytes < 10240 ? 1 : 0 })
  return bytes < 1024 ? t('quality.bytes', { n: bytes }) : bytes < 1 << 20 ? t('quality.kb', { n: n.format(bytes / 1024) }) : t('quality.mb', { n: n.format(bytes / (1 << 20)) })
}

function a11yDetail(t: PT, q: SiteQuality) {
  const a = q.a11y!
  const line = (c: A11yCounts) => IMPACTS.map((k) => `${t(`quality.impact.${k}`)} ${c[k]}`).join(', ')
  return [t('quality.a11yDesktop', { v: line(a.desktop) }), t('quality.a11yMobile', { v: line(a.mobile) }),
    a.top_rules.length > 0 ? t('quality.topRules', { v: a.top_rules.join(', ') }) : ''].filter(Boolean).join('\n')
}

// Objective signals about a site entry (accessibility, load, mobile fit). `full` lists them with details, the
// compact form is a row of small chips with the details in tooltips. They never affect the ranking.
export function QualitySummary({ quality: q, full = false }: { quality?: SiteQuality | null; full?: boolean }) {
  const t = usePT()
  if (!q || (!q.a11y && !q.perf && !q.mobile)) return null
  const w = q.a11y && worst(q)
  const issues = w ? sum(w) : 0
  const serious = w ? w.critical + w.serious : 0
  const chips = (
    <>
      {q.a11y && w && (
        <Badge variant={serious > 0 ? 'destructive' : issues > 0 ? 'outline' : 'secondary'} title={a11yDetail(t, q)}>
          {issues === 0 ? t('quality.a11yClean') : t('quality.a11yIssues', { n: issues, s: serious })}
        </Badge>
      )}
      {q.perf && (
        <Badge variant={q.perf.errors > 0 ? 'destructive' : 'secondary'} title={t('quality.perfHint', { dcl: q.perf.dcl_ms, load: q.perf.load_ms })}>
          {kb(t, q.perf.bytes)} · {t('quality.requests', { n: q.perf.requests })}{q.perf.errors > 0 && ` · ${t('quality.errors', { n: q.perf.errors })}`}
        </Badge>
      )}
      {q.mobile && (
        <Badge variant={q.mobile.overflow ? 'destructive' : 'secondary'} title={t('quality.mobileHint')}>
          {q.mobile.overflow ? t('quality.overflow') : t('quality.fits')}
          {q.mobile.small_targets > 0 && ` · ${t('quality.smallTargets', { n: q.mobile.small_targets })}`}
        </Badge>
      )}
    </>
  )
  if (!full) return <div className="flex flex-wrap gap-1.5">{chips}</div>
  return (
    <div className="mt-3 space-y-1.5 text-sm">
      <div className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{t('quality.title')}</div>
      <div className="flex flex-wrap gap-1.5">{chips}</div>
      {q.a11y && q.a11y.top_rules.length > 0 && <p className="break-words text-xs text-muted-foreground">{t('quality.topRules', { v: q.a11y.top_rules.join(', ') })}</p>}
      {q.perf && <p className="text-xs text-muted-foreground">{t('quality.perfHint', { dcl: q.perf.dcl_ms, load: q.perf.load_ms })}</p>}
      <p className="text-xs text-muted-foreground">{t('quality.noRank')}</p>
    </div>
  )
}
