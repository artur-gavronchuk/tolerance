import { HandleLink } from '@/components/daily/handle-link'
import { useBench } from './bench'
import { isBlind, usePT } from './phase'
import type { ProductEntry, ProductTask } from '@/lib/types'

const STEP = ['h-28', 'h-20', 'h-16'] // the first place stands tallest

// The top three, second place on the left and third on the right.
export function Podium({ task, entries }: { task: ProductTask; entries: ProductEntry[] }) {
  const t = usePT()
  const bench = useBench()
  const top = entries.slice(0, 3)
  if (top.length === 0) return null
  const order = top.length === 3 ? [1, 0, 2] : top.map((_, i) => i)
  return (
    <ol aria-label={t('pod.top3')}
      className={`grid items-end gap-2 border-b border-border sm:gap-4 ${top.length === 3 ? 'grid-cols-3' : top.length === 2 ? 'grid-cols-2' : 'mx-auto max-w-[10rem] grid-cols-1'}`}>
      {order.map((i) => {
        const e = top[i]
        return (
          <li key={e.id} className="min-w-0">
            <div className="px-1 pb-2 text-center">
              <div className="truncate font-semibold">{e.handle ? <HandleLink handle={e.handle} /> : '?'}{e.mine && <span className="font-normal text-muted-foreground">{t('gallery.you')}</span>}</div>
              <div className="truncate text-xs text-muted-foreground">
                {e.score != null && `${t('pod.pts', { n: Math.round(e.score) })} · `}{t.plural('votes', e.votes)}{e.total > 0 && !isBlind(task) && ` · ${e.passed}/${e.total}`}{e.bench_ms != null && !isBlind(task) && ` · ${bench.time(e.bench_ms)}`}
              </div>
            </div>
            <div className={`flex ${STEP[i]} items-center justify-center rounded-t-[14px] border border-b-0 ${i === 0 ? 'border-primary bg-primary/10 text-primary' : 'border-border bg-muted text-muted-foreground'}`}>
              <span className="display text-4xl">{i + 1}</span>
            </div>
            <span className="sr-only">{task.phase === 'final' ? t('pod.final') : t('pod.current')} {i + 1}</span>
          </li>
        )
      })}
    </ol>
  )
}
