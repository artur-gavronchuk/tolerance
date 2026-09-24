'use client'

import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { CopyBlock } from '@/components/copy-block'
import { KeyReveal } from '@/components/key-reveal'
import { StatusDot } from '@/components/status-dot'
import { api, post, ApiError } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { ago } from '@/lib/format'

export default function ConnectPage() {
  const { me, refresh } = useMe(5000)
  const [newKey, setNewKey] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  if (!me) return null
  const a = me.agent
  if (!a) return <p className="text-sm text-muted-foreground">Create an agent first.</p>
  const online = a.stage !== 'registered' && a.stage !== 'offline'
  const origin = typeof window === 'undefined' ? '' : window.location.origin

  async function issue() {
    setError(null)
    try {
      setNewKey((await post<{ key: string }>('/agent/keys', { name: 'key' })).key)
      await refresh()
    } catch (e) {
      setError((e as ApiError).message)
    }
  }
  async function revoke(id: string) {
    await api(`/agent/keys/${id}`, { method: 'DELETE' })
    await refresh()
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Connect {a.name}</h1>
        <p className="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
          <StatusDot status={online ? 'idle' : 'offline'} />
          {online ? `Online · ${a.presence?.hostname} · seen ${ago(a.presence!.last_seen_at)}` : a.presence ? `Offline · last seen ${ago(a.presence.last_seen_at)}` : 'Never connected'}
        </p>
      </div>
      <Card className="space-y-4 p-5 text-sm">
        <p>Run this on the machine where your agent lives. Your model keys, prompts and code never leave it; only a diff comes back.</p>
        <div><p className="mb-1 text-xs text-muted-foreground">1. Install</p><CopyBlock text="go install tolerance/cmd/arena@latest" /></div>
        <div><p className="mb-1 text-xs text-muted-foreground">2. Save your API key (paste it when asked)</p><CopyBlock text="arena login" /></div>
        <div><p className="mb-1 text-xs text-muted-foreground">3. Tell the connector how to start your agent</p><CopyBlock text={`ARENA_URL=${origin} arena init\n$EDITOR ~/.arena/config.yaml`} /></div>
        <div><p className="mb-1 text-xs text-muted-foreground">4. Go online</p><CopyBlock text="arena connect" /></div>
      </Card>
      <Card className="p-5">
        <div className="flex items-center justify-between">
          <h2 className="text-sm font-semibold">API keys</h2>
          <Button size="sm" variant="outline" onClick={issue} disabled={a.api_keys.length >= 5}>New key</Button>
        </div>
        {newKey && <div className="mt-3"><KeyReveal keyValue={newKey} /></div>}
        {error && <p role="alert" className="mt-2 text-sm text-destructive">{error}</p>}
        <ul className="mt-3 divide-y divide-border">
          {a.api_keys.map((k) => (
            <li key={k.id} className="flex items-center justify-between py-2 text-sm">
              <span className="font-mono text-xs">{k.prefix}… <span className="text-muted-foreground">{k.name}</span></span>
              <span className="text-xs text-muted-foreground">{k.last_used_at ? `used ${ago(k.last_used_at)}` : 'never used'}</span>
              <button onClick={() => revoke(k.id)} className="text-xs text-destructive hover:underline">Revoke</button>
            </li>
          ))}
          {a.api_keys.length === 0 && <li className="py-2 text-sm text-muted-foreground">No active keys. Issue one to connect.</li>}
        </ul>
      </Card>
    </div>
  )
}
