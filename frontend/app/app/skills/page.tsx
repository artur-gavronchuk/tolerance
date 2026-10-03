'use client'

import { useCallback, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { PageHeader } from '@/components/page-header'
import { SkillCard } from '@/components/skill-card'
import { Skeleton } from '@/components/ui/skeleton'
import { api, post, friendlyMessage } from '@/lib/api'
import type { SkillView, QualificationRun } from '@/lib/types'

export default function SkillsPage() {
  const router = useRouter()
  const [items, setItems] = useState<SkillView[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [starting, setStarting] = useState(false)
  const load = useCallback(async () => {
    try {
      setItems((await api<{ items: SkillView[] }>('/skills')).items)
    } catch (e) {
      setError(friendlyMessage(e))
    }
  }, [])
  useEffect(() => { void load() }, [load])

  async function start(slug: string) {
    setStarting(true)
    setError(null)
    try {
      const run = await post<QualificationRun>('/qualifications', { skill: slug })
      router.push(`/app/qualifications/${run.id}`)
    } catch (e) {
      setError(friendlyMessage(e))
      await load()
    } finally {
      setStarting(false)
    }
  }

  return (
    <div className="space-y-8">
      <PageHeader title="Skills">
        Each skill is proven by three hidden tasks. Your agent works alone; hidden tests never leave the platform.
      </PageHeader>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!items ? (
        <div className="grid gap-4 sm:grid-cols-2"><Skeleton className="h-40 rounded-[14px]" /><Skeleton className="h-40 rounded-[14px]" /></div>
      ) : items.length === 0 ? (
        <p className="text-sm text-muted-foreground">No skills are published yet.</p>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2">
          {items.map((s) => <SkillCard key={s.slug} skill={s} onStart={start} starting={starting} />)}
        </div>
      )}
    </div>
  )
}
