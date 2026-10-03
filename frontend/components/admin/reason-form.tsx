'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { errorText } from '@/lib/i18n/messages/errors'
import { useT } from '@/lib/i18n/client'
import { moderationMessages } from '@/lib/i18n/messages/admin-moderation'

// An inline reason field with Confirm / Cancel; run() performs the action with the typed reason.
export function ReasonForm({ run, onDone, onCancel, label }: { run: (reason: string) => Promise<unknown>; onDone: () => void; onCancel: () => void; label: string }) {
  const t = useT(moderationMessages)
  const [reason, setReason] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!reason.trim()) return
    setBusy(true)
    setError(null)
    try {
      await run(reason.trim())
      onDone()
    } catch (err) {
      setError(errorText(err as Error, t.locale))
      setBusy(false)
    }
  }
  return (
    <form onSubmit={submit} className="flex w-full min-w-0 flex-wrap items-center gap-2">
      <Input value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t('reason')} aria-label={t('reason')}
        maxLength={500} autoFocus className="h-8 min-w-0 flex-1 basis-40 text-sm" />
      <Button type="submit" size="sm" variant="destructive" disabled={busy || !reason.trim()}>{busy ? t('working') : label}</Button>
      <Button type="button" size="sm" variant="ghost" onClick={onCancel} disabled={busy}>{t('cancel')}</Button>
      {error && <p role="alert" className="w-full text-xs text-destructive">{error}</p>}
    </form>
  )
}
