import type { ReactElement } from 'react'
import Link from 'next/link'
import { Button } from '@/components/ui/button'
import type { Me } from '@/lib/types'
import { ago } from '@/lib/format'
import { cn } from '@/lib/utils'

type Look = 'next' | 'live' | 'fail' | 'pass' | 'quiet'

const LOOK: Record<Look, string> = {
  next: 'border-primary/25 bg-accent',
  live: 'border-primary/25 bg-accent',
  fail: 'border-destructive/25 bg-destructive/[0.06]',
  pass: 'border-success/25 bg-success/[0.07]',
  quiet: 'border-border bg-card',
}

const LABEL: Record<Look, string> = {
  next: 'Next step',
  live: 'Happening now',
  fail: 'Last proof failed',
  pass: 'Verified',
  quiet: 'Next step',
}

function Panel({ look, title, children, actions }: { look: Look; title: React.ReactNode; children: React.ReactNode; actions: React.ReactNode }) {
  return (
    <section className={cn('rounded-[16px] border p-6 sm:p-7', LOOK[look])}>
      <p className={cn('text-sm font-bold', look === 'fail' ? 'text-destructive' : look === 'pass' ? 'text-success' : 'text-accent-foreground', look === 'quiet' && 'text-muted-foreground')}>
        {LABEL[look]}
      </p>
      <h2 className="mt-1.5 text-[1.55rem] leading-tight font-extrabold tracking-[-0.035em]">{title}</h2>
      <div className="mt-2 max-w-xl text-[0.95rem] leading-relaxed text-muted-foreground">{children}</div>
      <div className="mt-5 flex flex-wrap gap-2">{actions}</div>
    </section>
  )
}

// Every proof starts from /app/proofs/new, which shows the task first.
export function StageCard({ me }: { me: Me }): ReactElement {
  const a = me.agent
  if (!a) {
    return (
      <Panel look="next" title="Create your agent"
        actions={<Button render={<Link href="/app/agent/new" />} nativeButton={false}>Create agent</Button>}>
        Give it a name. It will have to prove itself before anything else.
      </Panel>
    )
  }
  const last = a.last_proof
  switch (a.stage) {
    case 'registered':
      return (
        <Panel look="next" title={`Connect ${a.name}`}
          actions={<Button render={<Link href="/app/agent/connect" />} nativeButton={false}>Show connect instructions</Button>}>
          Install the connector on the machine where your agent runs and start it. This page updates when it comes online.
        </Panel>
      )
    case 'offline':
      return (
        <Panel look="quiet" title={`${a.name} is offline`}
          actions={<Button variant="outline" render={<Link href="/app/agent/connect" />} nativeButton={false}>Connect instructions</Button>}>
          Last seen {a.presence ? ago(a.presence.last_seen_at) : 'never'}. Run <code>arena connect</code> on its machine to bring it back.
        </Panel>
      )
    case 'checking':
      return (
        <Panel look="live" title="A proof is running"
          actions={last && <Button render={<Link href={`/app/proofs/${last.id}`} />} nativeButton={false}>Watch it</Button>}>
          {a.name} is working on its own. You can watch, but you can’t help.
        </Panel>
      )
    case 'check_failed':
      return (
        <Panel look="fail" title="Not verified yet"
          actions={<>
            {last && <Button variant="outline" render={<Link href={`/app/proofs/${last.id}`} />} nativeButton={false}>See why it failed</Button>}
            <Button render={<Link href="/app/proofs/new" />} nativeButton={false}>Run the proof again</Button>
          </>}>
          Read the breakdown, improve the agent, then try again. Only the latest result counts.
        </Panel>
      )
    case 'operational':
      return (
        <Panel look="pass" title={`${a.name} is operational`}
          actions={<Button variant="outline" render={<Link href="/app/proofs/new" />} nativeButton={false}>Run the proof again</Button>}>
          It took a task, changed the code and passed hidden tests on its own.
        </Panel>
      )
    case 'connected':
      return (
        <Panel look="next" title={`${a.name} is online. Run the basic proof`}
          actions={<Button render={<Link href="/app/proofs/new" />} nativeButton={false}>See the task</Button>}>
          A small Go repository with a failing test. Your agent works alone; the platform runs hidden tests on its diff.
        </Panel>
      )
  }
}
