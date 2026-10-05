'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Bot, CalendarClock, Check, Copy, Download, FileText, Scale, Trophy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Markdown } from '@/components/daily/markdown'
import { BuildUpload } from '@/components/build/build-upload'
import { BuildGallery } from '@/components/build/build-gallery'
import { api } from '@/lib/api'
import { FORMATS, daysLeft, tx } from '@/lib/build-kinds'
import { errorText } from '@/lib/format'
import { formatDate } from '@/lib/i18n/core'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildChallenge, BuildPage } from '@/lib/types'
import { cn } from '@/lib/utils'

type Tab = 'task' | 'agent'

export function BuildView({ slug }: { slug: string }) {
  const t = useT(buildMessages)
  const [page, setPage] = useState<BuildPage | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('task')

  const load = useCallback(async () => {
    try {
      setPage(await api<BuildPage>(`/builds/${encodeURIComponent(slug)}`))
      setError(null)
    } catch (e) {
      setError(errorText(e, t.locale))
    }
  }, [slug, t.locale])

  useEffect(() => { void load() }, [load])

  // While the viewer's entry is being tested, poll until it settles.
  const pending = page?.mine?.status === 'queued' || page?.mine?.status === 'running'
  useEffect(() => {
    if (!pending) return
    const id = setInterval(() => void load(), 3000)
    return () => clearInterval(id)
  }, [pending, load])

  if (error && !page) return <p role="alert" className="text-destructive">{error}</p>
  if (!page) return <div className="space-y-6"><Skeleton className="h-80 w-full rounded-[20px]" /><Skeleton className="h-64" /></div>

  const c = page.challenge
  const f = FORMATS[c.format]

  return (
    <div className="space-y-8">
      <Link href="/" className="-mb-4 inline-flex items-center gap-1.5 text-sm font-semibold text-muted-foreground hover:text-foreground">
        <ArrowLeft className="size-4" />{t('allChallenges')}
      </Link>
      <section className="relative overflow-hidden rounded-[20px] bg-terminal px-5 py-7 text-terminal-foreground sm:px-9 sm:py-10">
        <div aria-hidden className={cn('pointer-events-none absolute -right-24 -top-24 size-80 rounded-full blur-3xl', f.glow)} />
        <div aria-hidden className="pointer-events-none absolute -bottom-32 left-1/3 size-80 rounded-full bg-pop-1/25 blur-3xl" />
        <f.icon aria-hidden className="pointer-events-none absolute -right-8 top-6 size-56 rotate-12 text-white opacity-[0.06]" />
        <div className="relative">
          <div className="flex flex-wrap items-center gap-2 text-[13px] font-bold">
            <span className={cn('inline-flex items-center gap-1.5 rounded-full bg-gradient-to-r px-3 py-1 text-white', f.gradient)}>
              <f.icon className="size-3.5" />{t(`format_${c.format}`)}
            </span>
            <StatusLine c={c} />
          </div>
          <h1 className="display mt-4 max-w-3xl text-[2.6rem] sm:text-[4rem]">
            <span className={cn('bg-gradient-to-r bg-clip-text text-transparent', f.gradient)}>{tx(c.title, t.locale)}</span>
          </h1>
          <p className="mt-3 max-w-2xl text-[15px] leading-relaxed text-terminal-foreground/85 sm:text-xl">{tx(c.one_liner, t.locale)}</p>
          <div className="mt-6 flex flex-wrap items-center gap-3">
            <span className="inline-flex items-center gap-2 rounded-full border border-white/15 bg-white/[0.06] px-3.5 py-1.5 text-sm font-bold">
              <Scale className="size-4 text-terminal-accent" />{t('formula')}
            </span>
            {c.status !== 'upcoming' && (
              <span className="inline-flex items-center gap-2 text-sm text-terminal-foreground/70">
                <Trophy className="size-4" />{t.plural('solutions', c.entries)} · {t.plural('votes', c.votes)}
              </span>
            )}
          </div>
          <p className="mt-3 max-w-2xl text-sm text-terminal-foreground/60">{t('formulaHint')}</p>
        </div>
      </section>

      {c.status === 'upcoming' ? (
        <p className="rounded-[14px] border-2 border-dashed border-strong px-6 py-12 text-center text-lg font-semibold text-muted-foreground">
          {t('opensOn', { date: formatDate(t.locale, c.opens_at, { day: 'numeric', month: 'long' }) })}. {t('upcomingText')}
        </p>
      ) : (
        <>
          <div role="tablist" className="flex gap-1 border-b border-border">
            {(['task', 'agent'] as const).map((k) => (
              <button key={k} role="tab" aria-selected={tab === k} onClick={() => setTab(k)}
                className={cn('-mb-px flex items-center gap-2 border-b-2 px-4 py-2.5 text-[15px] font-bold transition-colors',
                  tab === k ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground')}>
                {k === 'task' ? <Trophy className="size-4" /> : <Bot className="size-4" />}
                {k === 'task' ? t('tabTask') : t('tabAgent')}
              </button>
            ))}
          </div>
          {tab === 'task' ? (
            <div className="space-y-10">
              {c.status === 'closed' ? (
                <p className="rounded-[14px] bg-muted px-5 py-4 font-semibold text-muted-foreground">{t('closedText')}</p>
              ) : (
                <section className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                  <BuildUpload slug={c.slug} mine={page.mine} onUploaded={() => void load()} />
                  <AgentTeaser onOpen={() => setTab('agent')} />
                </section>
              )}
              <BuildGallery challenge={c} entries={page.entries} reference={page.reference} onChange={() => void load()} />
            </div>
          ) : (
            <ForAgent c={c} />
          )}
        </>
      )}
    </div>
  )
}

function StatusLine({ c }: { c: BuildChallenge }) {
  const t = useT(buildMessages)
  const d = (iso: string) => formatDate(t.locale, iso, { day: 'numeric', month: 'long' })
  if (c.status === 'upcoming') return <span className="text-terminal-foreground/70">{t('opensOn', { date: d(c.opens_at) })}</span>
  if (c.status === 'closed') return <span className="text-terminal-foreground/70">{t('closedOn', { date: d(c.closes_at) })}</span>
  const left = daysLeft(c.closes_at)
  return (
    <span className="inline-flex items-center gap-1.5 text-terminal-foreground/80">
      <CalendarClock className="size-4" />{t('openUntil', { date: d(c.closes_at) })} · {left > 1 ? t.plural('daysLeft', left) : t('lastDay')}
    </span>
  )
}

function AgentTeaser({ onOpen }: { onOpen: () => void }) {
  const t = useT(buildMessages)
  return (
    <div className="flex flex-col justify-between gap-4 rounded-[14px] border border-border bg-card p-5">
      <div>
        <h2 className="heading flex items-center gap-2 text-lg"><Bot className="size-5 text-primary" />{t('tabAgent')}</h2>
        <p className="mt-2 text-sm text-muted-foreground">{t('promptHint')}</p>
        <ul className="mt-3 list-disc space-y-1.5 pl-5 text-sm">
          <li>{t('rule1')}</li>
          <li>{t('rule2')}</li>
          <li>{t('rule3')}</li>
        </ul>
      </div>
      <Button size="lg" onClick={onOpen} className="w-full sm:w-auto"><FileText />{t('toAgent')}</Button>
    </div>
  )
}

// ForAgent is the contract tab: the common rules, the contract and a prompt that carries both.
function ForAgent({ c }: { c: BuildChallenge }) {
  const t = useT(buildMessages)
  const contract = tx(c.contract, t.locale)
  const rules = `## ${t('rulesTitle')}\n\n- ${t('rule1')}\n- ${t('rule2')}\n- ${t('rule3')}\n`
  const prompt = `${t('promptIntro', { title: tx(c.title, t.locale), oneLiner: tx(c.one_liner, t.locale) })}\n\n${rules}\n${contract}`
  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <article className="min-w-0 rounded-[14px] border border-border bg-card p-5 sm:p-7">
        <Markdown>{rules}</Markdown>
        <hr className="my-6 border-border" />
        <Markdown>{contract}</Markdown>
      </article>
      <aside className="space-y-4 lg:sticky lg:top-6 lg:self-start">
        <div className="rounded-[14px] border border-border bg-card p-5">
          <h2 className="heading text-lg">{t('promptTitle')}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{t('promptHint')}</p>
          <pre className="mt-3 max-h-56 overflow-auto rounded-[10px] bg-terminal p-3 font-mono text-xs leading-5 whitespace-pre-wrap text-terminal-foreground">{prompt}</pre>
          <CopyButton text={prompt} />
        </div>
        <Button variant="outline" className="w-full" nativeButton={false}
          render={<a href={`/api/v1/builds/${encodeURIComponent(c.slug)}/task.zip`} download />}>
          <Download />{t('download')}
        </Button>
      </aside>
    </div>
  )
}

function CopyButton({ text }: { text: string }) {
  const t = useT(buildMessages)
  const [copied, setCopied] = useState(false)
  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1800)
    } catch {}
  }
  return (
    <Button size="lg" className="mt-3 w-full" onClick={() => void copy()}>
      {copied ? <Check /> : <Copy />}{copied ? t('copied') : t('copy')}
    </Button>
  )
}
