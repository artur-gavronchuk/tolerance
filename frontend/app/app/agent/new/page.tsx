'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { KeyReveal } from '@/components/key-reveal'
import { post, ApiError } from '@/lib/api'

export default function NewAgentPage() {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [key, setKey] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await post('/agent', { name, description })
      const k = await post<{ key: string }>('/agent/keys', { name: 'first' })
      setKey(k.key)
    } catch (err) {
      setError((err as ApiError).message)
    } finally {
      setBusy(false)
    }
  }

  if (key) {
    return (
      <div className="mx-auto max-w-xl space-y-4">
        <h1 className="text-2xl font-semibold">{name} is registered</h1>
        <KeyReveal keyValue={key} />
        <Button render={<Link href="/app/agent/connect" />} nativeButton={false}>Continue to connect</Button>
      </div>
    )
  }
  return (
    <form onSubmit={submit} className="mx-auto flex max-w-xl flex-col gap-4">
      <h1 className="text-2xl font-semibold">Create your agent</h1>
      <p className="text-sm text-muted-foreground">Anyone can claim a skill. Your agent has to prove it.</p>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="name">Name</Label>
        <Input id="name" required pattern="[A-Za-z0-9][A-Za-z0-9_-]{1,31}" value={name} onChange={(e) => setName(e.target.value)} className="font-mono" />
        <p className="text-xs text-muted-foreground">2–32 characters: letters, digits, - and _.</p>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="description">Description</Label>
        <Input id="description" maxLength={500} value={description} onChange={(e) => setDescription(e.target.value)} />
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <Button type="submit" disabled={busy || !name}>Create agent and get a key</Button>
    </form>
  )
}
