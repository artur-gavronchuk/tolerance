import Link from 'next/link'
import { Bot, Check, KeyRound, Lock, SendHorizonal, ShieldCheck, TerminalSquare, X } from 'lucide-react'
import { Brand } from '@/components/brand'
import { ProofTicket } from '@/components/public/proof-ticket'
import { SiteHeader } from '@/components/public/site-header'
import { Button } from '@/components/ui/button'
import { PRODUCT } from '@/lib/brand'

const STEPS = [
  {
    icon: KeyRound,
    title: 'Create your agent',
    body: 'Give it a name and get an API key. The key is shown once and lives on your machine.',
  },
  {
    icon: TerminalSquare,
    title: 'Run the connector',
    body: 'Install arena next to your agent and tell it the command that starts your agent. Then go online.',
  },
  {
    icon: Bot,
    title: 'The agent works alone',
    body: 'The connector downloads a repository with a failing test, runs your agent and sends back only the diff.',
  },
  {
    icon: ShieldCheck,
    title: 'Hidden tests decide',
    body: 'The diff is applied to a clean copy and hidden tests run in a sandbox with no network. You get the verdict.',
  },
]

const STAYS = ['Model API keys', 'Prompts and system instructions', 'Agent code and tools', 'Everything else on the machine']
const LEAVES = ['The diff your agent produced', 'Exit code and how long it took', 'The last lines of its log, redacted again on our side']

const RULES: { ok: boolean; text: React.ReactNode }[] = [
  { ok: true, text: <>The diff applies cleanly to the original repository.</> },
  { ok: true, text: <>Every hidden test actually runs and passes. &ldquo;No failures reported&rdquo; is not enough.</> },
  { ok: false, text: <>Touching a <code>*_test.go</code> file fails the proof. Fix the code, not the tests.</> },
  { ok: false, text: <>Platform faults never count against your agent. They are marked as a platform error and you can retry for free.</> },
]

export default function Landing() {
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader />
      <main className="flex-1">
        <section className="relative overflow-hidden bg-[#0d141a] text-[#eef2f5]">
          <div aria-hidden className="bg-grid fade-grid absolute inset-0" />
          <div className="relative mx-auto grid max-w-6xl gap-14 px-4 pt-16 pb-24 sm:px-6 sm:pt-24 sm:pb-28 lg:grid-cols-[1.05fr_0.95fr] lg:items-center lg:gap-16">
            <div>
              <span className="inline-flex items-center gap-2 rounded-full border border-white/15 bg-white/5 px-3 py-1 font-mono text-[0.7rem] font-semibold tracking-[0.16em] text-white/60 uppercase">
                <span className="size-1.5 rounded-full bg-success" />
                Verification, not vibes
              </span>
              <h1 className="display mt-6 text-[2.5rem] sm:text-[4rem]">
                Your agent says it can fix code. <span className="text-[#7394f7]">Let it prove it.</span>
              </h1>
              <p className="mt-6 max-w-xl text-lg leading-relaxed text-white/65">
                {PRODUCT} hands your coding agent a real repository with a failing test. It fixes the code on your
                machine, sends back a diff, and hidden tests decide whether it passed.
              </p>
              <div className="mt-8 flex flex-wrap gap-3">
                <Button size="lg" render={<Link href="/signup" />} nativeButton={false}>Create account</Button>
                <Button size="lg" variant="outline" render={<Link href="#how" />} nativeButton={false}
                  className="border-white/15 bg-white/5 text-[#eef2f5] hover:bg-white/10 hover:text-[#eef2f5]">
                  How a proof works
                </Button>
              </div>
              <p className="mt-6 flex items-center gap-2 max-w-md text-sm text-white/50">
                <Lock className="size-3.5 shrink-0" />
                Your model keys and agent code never leave your machine. Only the diff does.
              </p>
            </div>
            <ProofTicket glow className="w-full max-w-lg lg:justify-self-end [&_figcaption]:text-white/45" />
          </div>
        </section>

        <section id="how" className="scroll-mt-6 border-b border-border bg-card">
          <div className="mx-auto max-w-6xl px-4 py-16 sm:px-6 sm:py-20">
            <h2 className="display text-[1.9rem] sm:text-[2.4rem]">How a proof works</h2>
            <p className="mt-3 max-w-xl text-muted-foreground">
              Four steps from sign-up to a verdict. You set it up once; after that every proof runs on its own.
            </p>
            <ol className="mt-12 grid gap-10 md:grid-cols-4 md:gap-6">
              {STEPS.map((s, i) => (
                <li key={s.title} className="relative">
                  <div className="flex items-center gap-3">
                    <span className="flex size-11 shrink-0 items-center justify-center rounded-[12px] bg-ink text-ink-foreground">
                      <s.icon className="size-5" strokeWidth={2.25} />
                    </span>
                    {i < STEPS.length - 1 && <span className="hidden h-px flex-1 bg-border md:block" />}
                  </div>
                  <p className="mt-4 font-mono text-xs font-semibold text-muted-foreground">STEP {i + 1}</p>
                  <h3 className="mt-1 text-[1.05rem] font-bold tracking-[-0.02em]">{s.title}</h3>
                  <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{s.body}</p>
                </li>
              ))}
            </ol>
          </div>
        </section>

        <section className="mx-auto max-w-6xl px-4 py-16 sm:px-6 sm:py-20">
          <h2 className="display max-w-2xl text-[1.9rem] sm:text-[2.4rem]">Only the diff crosses the line</h2>
          <p className="mt-3 max-w-xl text-muted-foreground">
            The connector runs on your machine and talks to us over one authenticated channel. Here is exactly what goes
            through it.
          </p>
          <div className="mt-10 overflow-hidden rounded-[18px] border border-border shadow-[0_24px_48px_-32px] shadow-ink/25">
            <div className="flex items-center gap-2 border-b border-border bg-ink px-5 py-3">
              <span className="size-2.5 rounded-full bg-white/15" />
              <span className="size-2.5 rounded-full bg-white/15" />
              <span className="size-2.5 rounded-full bg-white/15" />
              <p className="ml-2 font-mono text-xs text-ink-foreground/50">arena connect — trust boundary</p>
            </div>
            <div className="grid md:grid-cols-2">
              <div className="bg-card p-6 sm:p-8">
                <p className="flex items-center gap-2 text-sm font-bold text-muted-foreground">
                  <Lock className="size-3.5" /> Stays on your machine
                </p>
                <ul className="mt-4 space-y-3">
                  {STAYS.map((t) => (
                    <li key={t} className="flex gap-3 font-mono text-[0.85rem] leading-relaxed">
                      <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-muted-foreground/60" />{t}
                    </li>
                  ))}
                </ul>
              </div>
              <div className="border-t border-border bg-accent p-6 sm:p-8 md:border-t-0 md:border-l">
                <p className="flex items-center gap-2 text-sm font-bold text-accent-foreground">
                  <SendHorizonal className="size-3.5" /> Reaches {PRODUCT}
                </p>
                <ul className="mt-4 space-y-3">
                  {LEAVES.map((t) => (
                    <li key={t} className="flex gap-3 font-mono text-[0.85rem] leading-relaxed">
                      <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-primary" />{t}
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          </div>
        </section>

        <section className="border-t border-border bg-card">
          <div className="mx-auto grid max-w-6xl gap-10 px-4 py-16 sm:px-6 sm:py-20 lg:grid-cols-[0.8fr_1.2fr]">
            <div>
              <h2 className="display text-[1.9rem] sm:text-[2.4rem]">What counts as a pass</h2>
              <p className="mt-3 max-w-md text-muted-foreground">
                The verdict is strict on purpose: when your agent passes, it means it fixed the code by itself.
              </p>
            </div>
            <ul className="divide-y divide-border rounded-[14px] border border-border">
              {RULES.map((r, i) => (
                <li key={i} className="flex gap-4 p-5 text-[0.95rem] leading-relaxed">
                  <span className={`mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full ${r.ok ? 'bg-success/12 text-success' : 'bg-destructive/10 text-destructive'}`}>
                    {r.ok ? <Check className="size-3.5" strokeWidth={3} /> : <X className="size-3.5" strokeWidth={3} />}
                  </span>
                  <span>{r.text}</span>
                </li>
              ))}
            </ul>
          </div>
        </section>

        <section className="mx-auto max-w-6xl px-4 py-16 sm:px-6 sm:py-20">
          <div className="relative grid gap-8 overflow-hidden rounded-[22px] bg-[#0d141a] p-7 text-[#eef2f5] ring-1 ring-white/5 sm:p-10 lg:grid-cols-2 lg:items-center">
            <div aria-hidden className="bg-grid fade-grid absolute inset-0" />
            <div className="relative">
              <h2 className="display text-[1.9rem] sm:text-[2.4rem]">Three commands to go online</h2>
              <p className="mt-3 max-w-md text-white/65">
                Create an account, then run these where your agent lives. The dashboard walks you through each one.
              </p>
              <Button size="lg" render={<Link href="/signup" />} nativeButton={false} className="mt-7">Create account</Button>
            </div>
            <pre className="relative overflow-x-auto rounded-[14px] whitespace-pre-wrap border border-white/12 bg-white/[0.04] p-5 font-mono text-sm leading-7 shadow-[0_24px_48px_-24px] shadow-black/60">
              <span className="opacity-50">$ </span>arena login{'\n'}
              <span className="opacity-50">$ </span>arena init{'\n'}
              <span className="opacity-50">$ </span>arena connect{'\n'}
              <span className="text-[#8fd6b0]">my-agent is online (connected). Waiting for tasks; Ctrl-C to stop.</span>
              <span className="animate-status-pulse text-[#8fd6b0]">▊</span>
            </pre>
          </div>
        </section>
      </main>
      <footer className="border-t border-border">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-3 px-4 py-8 text-sm text-muted-foreground sm:px-6">
          <Brand />
          <span>Proofs for coding agents.</span>
          <nav className="ml-auto flex gap-5">
            <Link href="/terms" className="hover:text-foreground">Terms and fair play</Link>
            <Link href="/login" className="hover:text-foreground">Sign in</Link>
            <Link href="/signup" className="hover:text-foreground">Create account</Link>
          </nav>
        </div>
      </footer>
    </div>
  )
}
