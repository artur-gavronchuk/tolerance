'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { notFound } from 'next/navigation'
import { Check, X } from 'lucide-react'
import { SectionTitle } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { StreakStrip } from '@/components/profile/streak-strip'
import { StackName, displayStack, localizeStack } from '@/components/daily/stack-label'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { dailyMessages } from '@/lib/i18n/messages/daily'
import { formatDate } from '@/lib/i18n/core'
import { api, ApiError } from '@/lib/api'
import { AccountManage } from '@/components/account/manage'
import { UploadLinkManage } from '@/components/upload-link/manage'
import { useMe } from '@/lib/use-me'
import type { Profile, ProfileDay } from '@/lib/types'

// The last 12 weeks as a grid of days, GitHub-style: solved, tried, or empty.
function Calendar({ days }: { days: ProfileDay[] }) {
  const t = useT(dailyMessages)
  const byDay = new Map(days.map((d) => [d.day, d]))
  const today = new Date()
  const cells: { day: string; d?: ProfileDay }[] = []
  for (let i = 83; i >= 0; i--) {
    const t = new Date(Date.UTC(today.getUTCFullYear(), today.getUTCMonth(), today.getUTCDate() - i))
    const day = t.toISOString().slice(0, 10)
    cells.push({ day, d: byDay.get(day) })
  }
  const played = cells.filter((c) => c.d).length
  return (
    <div className="flex flex-col gap-5 sm:flex-row sm:items-center sm:gap-8">
      <div className="grid w-full max-w-xs grid-flow-col grid-cols-12 grid-rows-7 gap-1 sm:max-w-sm">
        {cells.map(({ day, d }) => (
          <div key={day} title={d ? `${day}: ${d.passed_tests}/${d.total_tests}` : day}
            className={`aspect-square rounded-[4px] ${!d ? 'bg-muted' : d.status === 'passed' ? 'bg-success' : 'bg-primary/40'}`} />
        ))}
      </div>
      <div className="space-y-2 text-sm text-muted-foreground">
        <p><span className="font-mono font-bold text-foreground">{played}</span> {t('played84')}</p>
        <ul className="space-y-1 text-xs">
          <li className="flex items-center gap-2"><span className="size-3 rounded-[3px] bg-success" />{t('legendSolved')}</li>
          <li className="flex items-center gap-2"><span className="size-3 rounded-[3px] bg-primary/40" />{t('legendTried')}</li>
          <li className="flex items-center gap-2"><span className="size-3 rounded-[3px] bg-muted" />{t('legendNone')}</li>
        </ul>
      </div>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="rounded-xl border border-border bg-card p-4">
      <div className="font-mono text-2xl font-bold">{value}</div>
      <div className="text-xs text-muted-foreground">{label}</div>
    </div>
  )
}

export function ProfileView({ handle }: { handle: string }) {
  const t = useT(dailyMessages)
  const { me } = useMe()
  const [p, setP] = useState<Profile | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [missing, setMissing] = useState(false)
  useEffect(() => {
    api<Profile>(`/users/${encodeURIComponent(handle)}`).then(setP).catch((e) => {
      if (e instanceof ApiError && e.status === 404) setMissing(true)
      else setError(errorText(e, t.locale))
    })
  }, [handle])

  if (missing) notFound()
  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!p) return <Skeleton className="h-64 rounded-xl" />

  const isMe = me?.user.handle.toLowerCase() === p.handle.toLowerCase()
  const tools = [...new Set(p.tools.map((x) => localizeStack(displayStack(x), t.locale)))]

  return (
    <div className="space-y-10">
      <header className="space-y-2">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <h1 className="display text-title-sm break-words sm:text-title">{p.handle}</h1>
          {isMe && <Badge>{t('thisIsYou')}</Badge>}
        </div>
        <p className="text-sm text-muted-foreground">
          {t('joined', { date: formatDate(t.locale, p.joined_at) })}
          {tools.length > 0 && <> · {t('worksWith', { tools: tools.join(', ') })}</>}
        </p>
      </header>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <Stat label={t('placeOverall')} value={p.place ? `#${p.place}` : '—'} />
        <Stat label={t('currentStreak')} value={`🔥 ${p.streak.current}`} />
        <Stat label={t('bestStreak')} value={p.streak.best} />
        <Stat label={t('daysSolved')} value={`${p.solved_days}/${p.played_days}`} />
      </div>
      <div className="max-w-sm"><StreakStrip hideNumbers solved={p.days.filter((d) => d.status === 'passed').map((d) => d.day)} streak={p.streak} /></div>

      {isMe && <UploadLinkManage />}
      {isMe && <AccountManage handle={p.handle} />}

      <section>
        <SectionTitle>{t('last12')}</SectionTitle>
        <div className="rounded-xl border border-border bg-card p-4 sm:p-5"><Calendar days={p.days} /></div>
      </section>

      <section>
        <SectionTitle>{t('history')}</SectionTitle>
        {p.days.length === 0 ? (
          <p className="rounded-xl border border-dashed border-strong px-5 py-8 text-center text-sm text-muted-foreground">{t('noDaily')}</p>
        ) : (
          <ul className="divide-y divide-border rounded-xl border border-border bg-card">
            {p.days.map((d) => (
              <li key={d.day} className="flex flex-wrap items-center gap-x-3 gap-y-1 p-4">
                {d.status === 'passed' ? <Check className="size-4 shrink-0 text-success" /> : <X className="size-4 shrink-0 text-destructive" />}
                <span className="font-mono text-xs text-muted-foreground">{d.day}</span>
                <Link href={`/day/${d.day}`} className="min-w-0 flex-1 truncate font-semibold hover:text-primary hover:underline">{d.task.title}</Link>
                <span className="font-mono text-sm font-bold">
                  {d.score != null ? d.score : `${d.passed_tests}/${d.total_tests}`}
                </span>
                {d.made_with && <Badge variant="outline"><StackName madeWith={d.made_with} /></Badge>}
                <span className="text-xs text-muted-foreground">{t.plural('attempts', d.attempts)}</span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  )
}
