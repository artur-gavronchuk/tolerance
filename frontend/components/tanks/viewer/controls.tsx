import { Pause, Play } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useT } from '@/lib/i18n/client'
import { tanksMatchMessages } from '@/lib/i18n/messages/tanks-match'

const SPEEDS = [0.5, 1, 2, 4] as const

function mmss(tick: number, tickRate: number): string {
  const s = Math.max(0, Math.round(tick / tickRate))
  const m = Math.floor(s / 60)
  const r = s % 60
  return `${m}:${String(r).padStart(2, '0')}`
}

export function Controls({ playing, onPlayPause, speed, onSpeed, tick, maxTick, onSeek, live, tickRate }: {
  playing: boolean
  onPlayPause: () => void
  speed: number
  onSpeed: (x: number) => void
  tick: number
  maxTick: number
  onSeek: (tick: number) => void
  live: boolean
  tickRate: number
}) {
  const t = useT(tanksMatchMessages)
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-[12px] border border-border bg-card px-3 py-2.5">
      <button
        onClick={onPlayPause}
        aria-label={playing ? t('pause') : t('play')}
        className="flex size-8 shrink-0 items-center justify-center rounded-full bg-ink text-ink-foreground hover:bg-ink/85"
      >
        {playing ? <Pause className="size-3.5" fill="currentColor" /> : <Play className="size-3.5 translate-x-px" fill="currentColor" />}
      </button>
      {live ? (
        <span
          title={t('liveTitle')}
          className="flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2 text-xs"
        >
          <span className="flex items-center gap-1.5 font-bold text-primary">
            <span className="size-2 rounded-full bg-primary" />
            {t('latestMatch')}
          </span>
          <span className="text-muted-foreground">{t('sameMoment')}</span>
        </span>
      ) : (
        <>
        <span className="text-xs font-bold text-muted-foreground">{t('replay')}</span>
        <input
          type="range"
          min={0}
          max={Math.max(1, Math.round(maxTick))}
          step={1}
          value={Math.round(Math.min(tick, maxTick))}
          onChange={(e) => onSeek(Number(e.target.value))}
          aria-label={t('seek')}
          className="h-1.5 min-w-[6rem] flex-1 cursor-pointer accent-primary"
        />
        <span className="font-mono text-xs tabular-mono whitespace-nowrap text-muted-foreground">
          {mmss(tick, tickRate)} / {mmss(maxTick, tickRate)}
        </span>
        </>
      )}
      <div className="ml-auto flex gap-1">
        {SPEEDS.map((s) => (
          <button
            key={s}
            onClick={() => onSpeed(s)}
            aria-pressed={s === speed}
            className={cn(
              'rounded-md px-2 py-1 text-xs font-bold',
              s === speed ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-muted hover:text-foreground'
            )}
          >
            {s}×
          </button>
        ))}
      </div>
    </div>
  )
}
