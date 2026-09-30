'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { post, friendlyMessage } from '@/lib/api'
import type { MyBot } from '@/lib/types'

// Naming a bot is always optional — a new bot already gets an automatic
// name (its manifest's name, then the agent's name, then a random
// "tank-xxxxxx") the first time it's uploaded or the agent writes one. This
// form is how an owner picks something better, before or after that first
// version exists.
export function BotNameForm({ currentName, suggestedName, onSaved }: {
  currentName?: string
  suggestedName?: string
  onSaved: (bot: MyBot) => void
}) {
  const [name, setName] = useState(currentName ?? suggestedName ?? '')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const bot = await post<MyBot>('/me/tanks/bot', { name })
      onSaved(bot)
    } catch (err) {
      setError(friendlyMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-2 sm:flex-row sm:items-start sm:gap-2">
      <div className="flex-1 space-y-1.5">
        <Label htmlFor="bot-name" className="sr-only">Bot name</Label>
        <Input id="bot-name" required pattern="[A-Za-z0-9][A-Za-z0-9_-]{1,31}" value={name}
          onChange={(e) => setName(e.target.value)} className="font-mono" placeholder="my-tank" />
        {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
      </div>
      <Button type="submit" variant={currentName ? 'outline' : 'default'} disabled={busy || !name || name === currentName}>
        {busy ? 'Saving…' : currentName ? 'Rename' : 'Choose a name'}
      </Button>
    </form>
  )
}
