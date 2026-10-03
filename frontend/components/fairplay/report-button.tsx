'use client'

import { useState } from 'react'
import { Flag } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { fairplay } from '@/lib/api'
import { useMyHandle } from '@/lib/use-signed-in'
import { useT } from '@/lib/i18n/client'
import { errorText } from '@/lib/i18n/messages/errors'
import { fairplayMessages } from '@/lib/i18n/messages/fairplay'

const REASONS = ['cheating', 'copied', 'multi_account', 'offensive', 'other'] as const

// A small control next to a player's name: signed-in users pick a reason and send a report to the moderators.
export function ReportButton({ handle }: { handle: string }) {
  const t = useT(fairplayMessages)
  const me = useMyHandle()
  const [open, setOpen] = useState(false)
  const [done, setDone] = useState(false)
  const [reason, setReason] = useState<(typeof REASONS)[number]>('cheating')
  const [details, setDetails] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  if (!me || me.toLowerCase() === handle.toLowerCase()) return null
  if (done) return <span className="text-xs font-semibold text-muted-foreground">{t('sent')}</span>
  if (!open) {
    return (
      <Button type="button" size="xs" variant="ghost" className="text-muted-foreground" title={t('reportTitle')} onClick={() => setOpen(true)}>
        <Flag />{t('report')}
      </Button>
    )
  }
  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (reason === 'other' && !details.trim()) return
    setBusy(true)
    setError(null)
    try {
      await fairplay.report(handle, reason, details.trim())
      setDone(true)
    } catch (err) {
      setError((err as { code?: string }).code === 'state_conflict' ? t('already') : errorText(err as Error, t.locale))
      setBusy(false)
    }
  }
  return (
    <form onSubmit={submit} className="my-1 flex w-full max-w-sm flex-col gap-2 rounded-[10px] border border-border bg-card p-3 text-sm font-normal">
      <label className="flex flex-col gap-1">
        <span className="text-xs font-semibold text-muted-foreground">{t('reportReason')}</span>
        <select value={reason} onChange={(e) => setReason(e.target.value as (typeof REASONS)[number])}
          className="h-8 rounded-md border border-input bg-background px-2 text-sm">
          {REASONS.map((r) => <option key={r} value={r}>{t(`reason.${r}`)}</option>)}
        </select>
      </label>
      <label className="flex flex-col gap-1">
        <span className="text-xs font-semibold text-muted-foreground">{reason === 'other' ? t('reportDetailsRequired') : t('reportDetails')}</span>
        <textarea value={details} onChange={(e) => setDetails(e.target.value)} maxLength={500} rows={3}
          className="rounded-md border border-input bg-background px-2 py-1 text-sm" />
      </label>
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={busy || (reason === 'other' && !details.trim())}>{busy ? t('sending') : t('send')}</Button>
        <Button type="button" size="sm" variant="ghost" onClick={() => setOpen(false)} disabled={busy}>{t('cancel')}</Button>
      </div>
      {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
    </form>
  )
}
