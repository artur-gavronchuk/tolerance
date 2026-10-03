'use client'

import { useState } from 'react'
import { LockKeyhole } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { friendlyMessage, products } from '@/lib/api'
import type { ProductTask } from '@/lib/types'

// Admins (and anyone on a local dev run) can move a task's deadline: close it now to start voting, end the
// voting window, or open it again for a week, and start the next week's task early. Shown only when /me says
// can_admin. Without a task (the list page) only the rotation control shows.
export function AdminBar({ task, onChange }: { task?: ProductTask; onChange: () => void }) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function run(f: () => Promise<unknown>) {
    setBusy(true)
    setError(null)
    try {
      await f()
      onChange()
    } catch (e) {
      setError(friendlyMessage(e))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-[10px] border border-dashed border-input px-3 py-2 text-sm">
      <span className="inline-flex items-center gap-1.5 font-semibold text-muted-foreground"><LockKeyhole className="size-3.5" />Admin</span>
      {task?.phase === 'open' && (
        <Button size="sm" variant="outline" disabled={busy} onClick={() => void run(() => products.close(task.slug))}>Close now, start voting</Button>
      )}
      {task?.phase === 'voting' && (
        <Button size="sm" variant="outline" disabled={busy} onClick={() => void run(() => products.close(task.slug, true))}>Finalize now, end voting</Button>
      )}
      {task && task.phase !== 'open' && (
        <Button size="sm" variant="outline" disabled={busy} onClick={() => void run(() => products.reopen(task.slug, 7))}>Reopen for 7 days</Button>
      )}
      <Button size="sm" variant="outline" disabled={busy} onClick={() => void run(() => products.startNext())}>Start next week&apos;s task now</Button>
      {error && <span role="alert" className="text-destructive">{error}</span>}
    </div>
  )
}
