'use client'

import { useState } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { CopyBlock } from '@/components/copy-block'
import { KeyReveal } from '@/components/key-reveal'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { PRESENCE_LABEL, StatusDot } from '@/components/status-dot'
import { api, post, friendlyMessage } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { ago } from '@/lib/format'
import type { ApiKey } from '@/lib/types'
import { cn } from '@/lib/utils'

export default function ConnectPage() {
  const { me, refresh } = useMe(5000)
  const [newKey, setNewKey] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [revoking, setRevoking] = useState<ApiKey | null>(null)
  if (!me) return null
  const a = me.agent
  if (!a) {
    return (
      <p className="text-muted-foreground">
        Create an agent first. <Link href="/app/agent/new" className="font-semibold text-primary hover:underline">Create agent</Link>
      </p>
    )
  }
  const online = a.stage !== 'registered' && a.stage !== 'offline'
  const origin = typeof window === 'undefined' ? '' : window.location.origin
  const install = `curl -fsSL "${origin}/api/v1/connector/download?os=$(uname -s)&arch=$(uname -m)" -o arena\nchmod +x arena && sudo mkdir -p /usr/local/bin && sudo mv arena /usr/local/bin/arena`

  const steps = [
    {
      title: 'Install the connector',
      body: 'On the machine where your agent runs. macOS or Linux.',
      cmd: install,
      note: <>A 404 means this server has no prebuilt connector for your OS and architecture. Build it from the repository instead: <code>cd backend &amp;&amp; go build -o arena ./cmd/arena</code></>,
    },
    { title: 'Save your API key', body: 'Paste the key when it asks. It is stored in ~/.arena.', cmd: 'arena login' },
    { title: 'Tell it how to start your agent', body: 'Set agent.command in the config it writes. It runs with sh -c inside the task repository.', cmd: `ARENA_URL=${origin} arena init\n$EDITOR ~/.arena/config.yaml` },
    { title: 'Go online', body: 'Leave it running. This page turns green when it connects.', cmd: 'arena connect' },
  ]

  async function issue() {
    setError(null)
    try {
      setNewKey((await post<{ key: string }>('/agent/keys', { name: 'key' })).key)
      await refresh()
    } catch (e) {
      setError(friendlyMessage(e))
    }
  }
  async function revoke(id: string) {
    try {
      await api(`/agent/keys/${id}`, { method: 'DELETE' })
      await refresh()
    } catch (e) {
      setError(friendlyMessage(e))
    } finally {
      setRevoking(null)
    }
  }

  return (
    <div className="space-y-10">
      <PageHeader
        kicker={<span className="flex items-center gap-2"><StatusDot stage={a.stage} />
          {online ? `${PRESENCE_LABEL[a.stage]} on ${a.presence?.hostname}, seen ${ago(a.presence!.last_seen_at)}` : a.presence ? `Offline, last seen ${ago(a.presence.last_seen_at)}` : 'Never connected'}
        </span>}
        title={`Connect ${a.name}`}>
        Your model keys, prompts and code never leave that machine. Only the diff comes back.
      </PageHeader>

      <div className="grid gap-10 lg:grid-cols-[1fr_22rem]">
        <ol className="min-w-0 space-y-8">
          {steps.map((s, i) => {
            const last = i === steps.length - 1
            return (
              <li key={s.title} className="grid grid-cols-[2.25rem_1fr] gap-4">
                <span className={cn('flex size-9 items-center justify-center rounded-full font-mono text-sm font-bold',
                  last && online ? 'bg-success text-success-foreground' : 'bg-ink text-ink-foreground')}>{i + 1}</span>
                <div className="min-w-0 space-y-2.5">
                  <div>
                    <h2 className="heading text-[1.05rem]">{s.title}</h2>
                    <p className="text-sm text-muted-foreground">{s.body}</p>
                  </div>
                  <CopyBlock text={s.cmd} />
                  {s.note && <p className="text-xs leading-relaxed text-muted-foreground">{s.note}</p>}
                  {last && online && <p className="text-sm font-bold text-success">Connected. <Link href="/app/proofs/new" className="text-primary hover:underline">See the proof task</Link></p>}
                </div>
              </li>
            )
          })}
        </ol>

        <aside className="space-y-4">
          <section className="rounded-[14px] border border-border bg-card p-5">
            <SectionTitle aside={`${a.api_keys.length} of 5`} className="mb-1">API keys</SectionTitle>
            <p className="text-sm text-muted-foreground">One per machine is a good habit. Revoking a key disconnects that connector.</p>
            {newKey && <div className="mt-4"><KeyReveal keyValue={newKey} /></div>}
            {error && <p role="alert" className="mt-3 text-sm text-destructive">{error}</p>}
            <ul className="mt-4 divide-y divide-border rounded-[10px] border border-border">
              {a.api_keys.map((k) => (
                <li key={k.id} className="flex items-center gap-3 px-3 py-2.5 text-sm">
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-mono text-xs font-semibold">{k.prefix}…</p>
                    <p className="text-xs text-muted-foreground">{k.name}, {k.last_used_at ? `used ${ago(k.last_used_at)}` : 'never used'}</p>
                  </div>
                  <Button variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" onClick={() => setRevoking(k)}>Revoke</Button>
                </li>
              ))}
              {a.api_keys.length === 0 && <li className="px-3 py-3 text-sm text-muted-foreground">No active keys. Issue one to connect.</li>}
            </ul>
            <Button variant="outline" className="mt-4 w-full" onClick={issue} disabled={a.api_keys.length >= 5}>Issue a new key</Button>
          </section>
        </aside>
      </div>

      <Dialog open={revoking != null} onOpenChange={(open) => { if (!open) setRevoking(null) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Revoke {revoking?.prefix}…?</DialogTitle>
            <DialogDescription>A connector using this key stops working right away. This can’t be undone; you can issue a new key.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose render={<Button variant="outline" />}>Keep it</DialogClose>
            <Button variant="destructive" onClick={() => revoking && void revoke(revoking.id)}>Revoke key</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
