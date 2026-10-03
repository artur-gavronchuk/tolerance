'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { PageHeader } from '@/components/page-header'
import { TaskBadges } from '@/components/daily/task-header'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import type { DayListItem } from '@/lib/types'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { listingMessages } from '@/lib/i18n/messages/listings'

export default function DaysPage() {
  const t = useT(listingMessages)
  const [items, setItems] = useState<DayListItem[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    api<{ items: DayListItem[] }>('/days').then((r) => setItems(r.items)).catch((e) => setError(errorText(e, t.locale)))
  }, [])
  return (
    <div className="space-y-8">
      <PageHeader title={t('archiveTitle')}>{t('archiveIntro')}</PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!items && !error && <Skeleton className="h-64 rounded-[14px]" />}
      {items && items.length === 0 && (
        <div className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">
          <p>{t('archiveEmpty')}</p>
          <Link href="/" className="mt-3 inline-block font-semibold text-primary hover:underline">{t('backToToday')}</Link>
        </div>
      )}
      {items && items.length > 0 && (
        <ul className="divide-y divide-border rounded-[14px] border border-border bg-card">
          {items.map((d) => (
            <li key={d.day}>
              <Link href={`/day/${d.day}`} className="flex flex-wrap items-center gap-x-4 gap-y-2 p-4 hover:bg-muted/50">
                <span className="w-24 shrink-0 font-mono text-sm text-muted-foreground">{d.day}</span>
                <span className="min-w-0 flex-1 basis-40 truncate font-semibold">{d.task.title}</span>
                <TaskBadges language={d.task.language} difficulty={d.task.difficulty} />
                <span className="w-24 text-right text-sm text-muted-foreground">{t('solvedN', { n: d.solvers })}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
