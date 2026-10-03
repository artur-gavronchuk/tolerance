'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { friendlyMessage, tanks } from '@/lib/api'
import { useMe } from '@/lib/use-me'

// Starts an extra tournament right now. The API only allows it for admins, or for anyone signed in when the
// dev login is on (local runs), so the button is only offered to an admin, or on a dev build of the site.
export function StartTournament() {
  const { me } = useMe()
  const router = useRouter()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  if (!me || (process.env.NODE_ENV === 'production' && me.user.role !== 'admin')) return null

  async function start() {
    setBusy(true)
    setError(null)
    try {
      const t = await tanks.startTournament()
      router.push(`/tanks/tournaments/${t.id}`)
    } catch (e) {
      setError(friendlyMessage(e))
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col items-start gap-1.5 sm:items-end">
      <Button variant="outline" onClick={() => void start()} disabled={busy}>
        {busy ? 'Starting…' : 'Start a tournament now'}
      </Button>
      {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
    </div>
  )
}
