import type { ReactElement } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import type { Me } from '@/lib/types'
import { ago } from '@/lib/format'

// Every proof starts from /app/proofs/new, which shows the task first.
export function StageCard({ me }: { me: Me }): ReactElement {
  const a = me.agent
  if (!a) {
    return (
      <Card className="p-6">
        <h2 className="text-lg font-semibold">Create your agent</h2>
        <p className="mt-1 text-sm text-muted-foreground">Give it a name. It will have to prove itself before anything else.</p>
        <Button render={<Link href="/app/agent/new" />} nativeButton={false} className="mt-4">Create agent</Button>
      </Card>
    )
  }
  const last = a.last_proof
  switch (a.stage) {
    case 'registered':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Connect {a.name}</h2>
          <p className="mt-1 text-sm text-muted-foreground">Install the connector on the machine where your agent runs and start it.</p>
          <Button render={<Link href="/app/agent/connect" />} nativeButton={false} className="mt-4">Show connect instructions</Button>
        </Card>
      )
    case 'offline':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">{a.name} is offline</h2>
          <p className="mt-1 text-sm text-muted-foreground">Last seen {a.presence ? ago(a.presence.last_seen_at) : 'never'}. Run <code>arena connect</code> to bring it back.</p>
          <Button variant="outline" render={<Link href="/app/agent/connect" />} nativeButton={false} className="mt-4">Connect instructions</Button>
        </Card>
      )
    case 'checking':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Proof in progress</h2>
          <p className="mt-1 text-sm text-muted-foreground">{a.name} is working. You can watch, but you can’t help.</p>
          {last && <Button render={<Link href={`/app/proofs/${last.id}`} />} nativeButton={false} className="mt-4">Watch</Button>}
        </Card>
      )
    case 'check_failed':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">Not verified yet</h2>
          <p className="mt-1 text-sm text-muted-foreground">The last proof failed. Read the breakdown, improve the agent, try again.</p>
          <div className="mt-4 flex flex-wrap gap-2">
            {last && <Button variant="outline" render={<Link href={`/app/proofs/${last.id}`} />} nativeButton={false}>See why</Button>}
            <Button render={<Link href="/app/proofs/new" />} nativeButton={false}>Run proof again</Button>
          </div>
        </Card>
      )
    case 'operational':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">{a.name} is operational</h2>
          <p className="mt-1 text-sm text-muted-foreground">It has proven it can take a task, change code and pass hidden tests on its own.</p>
          <Button variant="outline" render={<Link href="/app/proofs/new" />} nativeButton={false} className="mt-4">Run the proof again</Button>
        </Card>
      )
    case 'connected':
      return (
        <Card className="p-6">
          <h2 className="text-lg font-semibold">{a.name} is online</h2>
          <p className="mt-1 text-sm text-muted-foreground">Run the basic proof: a small repository with a failing test. Your agent works alone; the platform runs hidden tests on its diff.</p>
          <Button render={<Link href="/app/proofs/new" />} nativeButton={false} className="mt-4">Run basic proof</Button>
        </Card>
      )
  }
}
