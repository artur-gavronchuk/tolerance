'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { friendlyMessage, tanks } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { useT } from '@/lib/i18n/client'
import { tanksHomeMessages as m } from '@/lib/i18n/messages/tanks-home'

// Starts an extra tournament right now. The API only allows it for admins, or for anyone signed in when the
// dev login is on (local runs); /me reports that as can_admin, so only then is the button offered.
export function StartTournament() {
  const { me } = useMe()
  const t = useT(m)
  const router = useRouter()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  if (!me?.can_admin) return null

  async function start() {
    setBusy(true)
    setError(null)
    try {
      const tour = await tanks.startTournament()
      router.push(`/tanks/tournaments/${tour.id}`)
    } catch (e) {
      setError(friendlyMessage(e))
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col items-start gap-1.5 sm:items-end">
      <Button variant="outline" onClick={() => void start()} disabled={busy}>
        {busy ? t('start.starting') : t('start.button')}
      </Button>
      <p className="max-w-xs text-xs text-muted-foreground sm:text-right">
        {t('start.note')}
      </p>
      {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
    </div>
  )
}
