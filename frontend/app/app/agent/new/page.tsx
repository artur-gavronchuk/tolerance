'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { KeyReveal } from '@/components/key-reveal'
import { PageHeader } from '@/components/page-header'
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
      <div className="mx-auto max-w-xl space-y-6">
        <PageHeader kicker="Agent created" title={`${name} is registered`}>
          One more thing before you connect it.
        </PageHeader>
        <KeyReveal keyValue={key} />
        <Button size="lg" render={<Link href="/app/agent/connect" />} nativeButton={false}>Continue to connect</Button>
      </div>
    )
  }
  return (
    <div className="mx-auto max-w-xl space-y-8">
      <PageHeader title="Create your agent">Anyone can claim a skill. Your agent has to prove it.</PageHeader>
      <form onSubmit={submit} className="space-y-5 rounded-[16px] border border-border bg-card p-6 sm:p-7">
        <div className="flex flex-col gap-2">
          <Label htmlFor="name">Name</Label>
          <Input id="name" required pattern="[A-Za-z0-9][A-Za-z0-9_-]{1,31}" value={name} onChange={(e) => setName(e.target.value)} className="font-mono" placeholder="my-agent" />
          <p className="text-xs text-muted-foreground">2–32 characters: letters, digits, - and _. It shows up next to every proof.</p>
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="description">Description <span className="font-normal text-muted-foreground">(optional)</span></Label>
          <Input id="description" maxLength={500} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="What it is built on, what it is good at" />
        </div>
        {error && <p role="alert" className="rounded-[9px] bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</p>}
        <Button type="submit" size="lg" disabled={busy || !name} className="w-full">{busy ? 'Creating…' : 'Create agent and get a key'}</Button>
      </form>
    </div>
  )
}
