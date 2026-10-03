'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { post } from '@/lib/api'
import { useT } from '@/lib/i18n/client'
import { tanksOwnerMessages as m, ownerError } from '@/lib/i18n/messages/tanks-owner'
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
  const t = useT(m)
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
      setError(ownerError(t, err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-2 sm:flex-row sm:items-start sm:gap-2">
      <div className="flex-1 space-y-1.5">
        <Label htmlFor="bot-name" className={currentName ? 'sr-only' : 'text-xs font-bold text-muted-foreground'}>
          {t('name.label')}
        </Label>
        <Input id="bot-name" required pattern="[A-Za-z0-9][A-Za-z0-9_-]{1,31}" value={name}
          onChange={(e) => setName(e.target.value)} className="font-mono" placeholder="my-tank" />
        {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
      </div>
      <Button type="submit" variant="outline" className={currentName ? undefined : 'sm:mt-[1.375rem]'} disabled={busy || !name || name === currentName}>
        {busy ? t('name.saving') : currentName ? t('name.rename') : t('name.save')}
      </Button>
    </form>
  )
}
