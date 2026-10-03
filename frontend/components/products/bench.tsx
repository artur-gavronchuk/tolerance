'use client'

import { usePT } from './phase'
import type { ProductEntry } from '@/lib/types'

// The best (smallest) benchmark time among entries, null when nobody has one.
export function fastestOf(entries: Pick<ProductEntry, 'bench_ms'>[]): number | null {
  const times = entries.map((e) => e.bench_ms).filter((m): m is number => m != null)
  return times.length > 0 ? Math.min(...times) : null
}

// "420 ms" / "1.25 s" and "×1.8" against the fastest entry, in the viewer's language.
export function useBench() {
  const t = usePT()
  const num = (n: number, digits: number, min = 0) => new Intl.NumberFormat(t.locale, { maximumFractionDigits: digits, minimumFractionDigits: min }).format(n)
  return {
    // with a spread: "420 ± 18 ms" / "1.25 ± 0.04 s"
    time: (ms: number, spread?: number | null) => {
      const pm = (k: number, digits: number) => (spread != null && spread > 0 ? `${num(ms / k, digits)} ± ${num(spread / k, digits)}` : num(ms / k, digits))
      return ms < 1000 ? t('bench.ms', { n: pm(1, 0) }) : t('bench.s', { n: pm(1000, 2) })
    },
    // null for the fastest itself (it gets the "fastest" label instead) or without a reference
    rel: (ms: number, fastest: number | null) => (fastest != null && ms > fastest * 1.005 ? t('bench.rel', { x: num(ms / fastest, 1, 1) }) : null),
  }
}

// An entry's benchmark time with how it compares to the fastest; "no time" when the tool earned none.
export function BenchTime({ ms, spread = null, fastest, className }: { ms: number | null; spread?: number | null; fastest: number | null; className?: string }) {
  const t = usePT()
  const b = useBench()
  if (ms == null) return <span className={className} title={t('bench.noneHint')}>{t('bench.none')}</span>
  const rel = b.rel(ms, fastest)
  return (
    <span className={`${className ?? ''} break-words`} title={t('bench.hint')}>
      {b.time(ms, spread)}
      {rel ? <span className="text-muted-foreground"> {rel}</span> : fastest != null && <span className="text-primary"> {t('bench.fastest')}</span>}
    </span>
  )
}
