'use client'

import Link from 'next/link'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { SectionTitle } from '@/components/page-header'
import { api } from '@/lib/api'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { accountMessages as m } from '@/lib/i18n/messages/account'

// Owner-only "Your data" block on the profile: JSON export and account deletion (confirmed by typing the handle).
export function AccountManage({ handle }: { handle: string }) {
  const t = useT(m)
  const [asking, setAsking] = useState(false)
  const [typed, setTyped] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)

  async function remove() {
    setBusy(true)
    setError(null)
    try {
      await api('/me', { method: 'DELETE', body: JSON.stringify({ handle: typed.trim() }) })
      window.location.assign('/')
    } catch (e) {
      setError(e)
      setBusy(false)
    }
  }

  return (
    <section>
      <SectionTitle>{t('title')}</SectionTitle>
      <div className="space-y-3 rounded-[14px] border border-border bg-card p-4 sm:p-5">
        <p className="max-w-2xl text-sm text-muted-foreground">{t('intro')} <Link className="text-foreground underline underline-offset-2" href="/privacy">{t('privacy')}</Link></p>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" nativeButton={false} render={<a href="/api/v1/me/export" download />}>{t('download')}</Button>
          {!asking && <Button size="sm" variant="outline" onClick={() => setAsking(true)}>{t('deleteButton')}</Button>}
        </div>
        {asking && (
          <div className="max-w-md space-y-3 rounded-[10px] border border-destructive/40 p-3">
            <p className="text-sm text-muted-foreground">{t('deleteWarn', { handle })}</p>
            <Label htmlFor="confirm-handle">{t('handleLabel')}</Label>
            <Input id="confirm-handle" autoComplete="off" value={typed} onChange={(e) => setTyped(e.target.value)} />
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="destructive" disabled={busy || typed.trim().toLowerCase() !== handle.toLowerCase()} onClick={() => void remove()}>
                {busy ? t('deleting') : t('confirm')}
              </Button>
              <Button size="sm" variant="outline" disabled={busy} onClick={() => { setAsking(false); setTyped('') }}>{t('cancel')}</Button>
            </div>
            {error != null && <p role="alert" className="text-sm text-destructive">{t('failed', { error: errorText(error, t.locale) })}</p>}
          </div>
        )}
      </div>
    </section>
  )
}
