'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft, Check, Copy, Download, Heart, Lock, Trophy } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Markdown } from '@/components/daily/markdown'
import { BuildUpload } from '@/components/build/build-upload'
import { BuildGallery, Thumb } from '@/components/build/build-gallery'
import { api } from '@/lib/api'
import { FORMATS, daysLeft, tx } from '@/lib/build-kinds'
import { errorText } from '@/lib/format'
import { formatDate } from '@/lib/i18n/core'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildChallenge, BuildEntry, BuildPage } from '@/lib/types'
import { cn } from '@/lib/utils'

type Tab = 'gallery' | 'contract'

export function BuildView({ slug }: { slug: string }) {
  const t = useT(buildMessages)
  const [page, setPage] = useState<BuildPage | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [tab, setTab] = useState<Tab>('gallery')
  const tabsRef = useRef<HTMLDivElement>(null)

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
  if (!page) return <div className="space-y-6"><Skeleton className="h-96 w-full rounded-[28px]" /><Skeleton className="h-64 rounded-[20px]" /></div>

  const c = page.challenge
  const showcase = page.entries[0] ?? page.reference

  function openContract() {
    setTab('contract')
    tabsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <div className="space-y-8">
      <Link href="/" className="-mb-3 inline-flex items-center gap-1.5 text-sm font-semibold text-muted-foreground hover:text-foreground">
        <ArrowLeft className="size-4" />{t('allChallenges')}
      </Link>
      <Hero c={c} showcase={c.status === 'upcoming' ? null : showcase} />

      {c.status === 'upcoming' ? (
        <p className="flex items-center justify-center gap-2 rounded-[20px] border-2 border-dashed border-strong px-6 py-12 text-center text-lg font-semibold text-muted-foreground">
          <Lock className="size-5" />{t('upcomingText')}
        </p>
      ) : (
        <>
          {c.status === 'closed' ? (
            <p className="rounded-[20px] bg-muted px-6 py-5 font-semibold text-muted-foreground">{t('closedText')}</p>
          ) : (
            <section className="grid gap-5 lg:grid-cols-2">
              <GiveContract c={c} onRead={openContract} />
              <Step n={2} title={page.mine ? t('uploadTitle') : t('step2Title')}>
                <BuildUpload slug={c.slug} mine={page.mine} onUploaded={() => void load()} />
              </Step>
            </section>
          )}

          <div ref={tabsRef} className="scroll-mt-6 space-y-6">
            <div role="tablist" className="flex gap-1 rounded-full bg-muted p-1 sm:w-fit">
              {(['gallery', 'contract'] as const).map((k) => (
                <button key={k} role="tab" aria-selected={tab === k} onClick={() => setTab(k)}
                  className={cn('flex-1 rounded-full px-5 py-2 text-[15px] font-bold transition-colors sm:flex-none',
                    tab === k ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground')}>
                  {k === 'gallery' ? `${t('gallery')} ${page.entries.length}` : t('contractTitle')}
                </button>
              ))}
            </div>
            {tab === 'gallery'
              ? <BuildGallery challenge={c} entries={page.entries} reference={page.reference} onChange={() => void load()} />
              : <Contract c={c} />}
          </div>
        </>
      )}
    </div>
  )
}

// Hero is the challenge's poster: its format colour, the title, the clock and, on wide screens, the best work so far.
function Hero({ c, showcase }: { c: BuildChallenge; showcase: BuildEntry | null }) {
  const t = useT(buildMessages)
  const f = FORMATS[c.format]
  const upcoming = c.status === 'upcoming'
  const mobile = c.format === 'mobile'
  return (
    <section className={cn('relative grid grid-cols-[minmax(0,1fr)] overflow-hidden rounded-[28px] lg:grid-cols-[minmax(0,1fr)_26rem]',
      upcoming ? 'bg-muted text-foreground' : cn('text-terminal', f.bg))}>
      <div className="relative z-10 p-6 sm:p-10">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm font-bold">
          <span className="flex items-center gap-1.5"><f.icon className="size-4" />{t(`format_${c.format}`)}</span>
          <StatusLine c={c} />
        </div>
        <h1 className="poster mt-6 max-w-3xl text-[2.3rem] sm:text-[4.4rem]">{tx(c.title, t.locale)}</h1>
        <p className="mt-4 max-w-xl text-[17px] font-medium leading-snug opacity-80 sm:text-xl">{tx(c.one_liner, t.locale)}</p>
        {!upcoming && (
          <div className="mt-6 flex flex-wrap items-center gap-x-5 gap-y-2 text-sm font-bold">
            <span className="flex items-center gap-1.5"><Trophy className="size-4" />{t.plural('solutions', c.entries)}</span>
            <span className="flex items-center gap-1.5"><Heart className="size-4" />{t.plural('votes', c.votes)}</span>
            <span className="opacity-70">{t('formula')}</span>
          </div>
        )}
      </div>
      {showcase && (
        <div aria-hidden className="relative hidden items-end justify-center pt-10 lg:flex">
          <div className={cn('translate-y-6 overflow-hidden border-terminal bg-terminal shadow-2xl',
            mobile ? 'aspect-[390/720] w-56 rounded-t-[32px] border-[7px] border-b-0' : 'aspect-[16/10] w-[115%] -mr-[15%] rounded-tl-[18px] border-[6px] border-b-0 border-r-0')}>
            <Thumb e={showcase} />
          </div>
        </div>
      )}
    </section>
  )
}

// GiveContract is step one: the prompt to copy (rules plus contract) or the same as a zip.
function GiveContract({ c, onRead }: { c: BuildChallenge; onRead: () => void }) {
  const t = useT(buildMessages)
  const { prompt } = usePrompt(c)
  return (
    <Step n={1} title={t('step1Title')}>
      <p className="text-muted-foreground">{t('step1Text')}</p>
      <div className="mt-5 flex flex-wrap gap-2">
        <CopyButton text={prompt} />
        <Button size="lg" variant="outline" nativeButton={false}
          render={<a href={`/api/v1/builds/${encodeURIComponent(c.slug)}/task.zip`} download />}>
          <Download />{t('download')}
        </Button>
      </div>
      <button onClick={onRead} className="mt-4 text-sm font-semibold text-primary underline-offset-4 hover:underline">
        {t('readContract')}
      </button>
    </Step>
  )
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <div className="rounded-[20px] border border-border bg-card p-5 sm:p-7">
      <h2 className="heading flex items-center gap-3 text-xl">
        <span className="poster flex size-9 shrink-0 items-center justify-center rounded-full bg-ink text-base text-ink-foreground">{n}</span>
        {title}
      </h2>
      <div className="mt-4">{children}</div>
    </div>
  )
}

function StatusLine({ c }: { c: BuildChallenge }) {
  const t = useT(buildMessages)
  const d = (iso: string) => formatDate(t.locale, iso, { day: 'numeric', month: 'long' })
  if (c.status === 'upcoming') return <span>{t('opensOn', { date: d(c.opens_at) })}</span>
  if (c.status === 'closed') return <span>{t('closedOn', { date: d(c.closes_at) })}</span>
  const left = daysLeft(c.closes_at)
  return <span>{t('openUntil', { date: d(c.closes_at) })}, {left > 1 ? t.plural('daysLeft', left) : t('lastDay')}</span>
}

// usePrompt is what the viewer pastes into their agent: the intro, the common rules and the contract.
function usePrompt(c: BuildChallenge) {
  const t = useT(buildMessages)
  const rules = `## ${t('rulesTitle')}\n\n- ${t('rule1')}\n- ${t('rule2')}\n- ${t('rule3')}\n`
  const prompt = `${t('promptIntro', { title: tx(c.title, t.locale), oneLiner: tx(c.one_liner, t.locale) })}\n\n${rules}\n${tx(c.contract, t.locale)}`
  return { rules, prompt }
}

// Contract is the full text the hidden tests check, with the prompt that carries it on the side.
function Contract({ c }: { c: BuildChallenge }) {
  const t = useT(buildMessages)
  const { rules, prompt } = usePrompt(c)
  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_22rem]">
      <article className="min-w-0 rounded-[20px] border border-border bg-card p-5 sm:p-8">
        <Markdown>{rules}</Markdown>
        <hr className="my-6 border-border" />
        <Markdown>{tx(c.contract, t.locale)}</Markdown>
      </article>
      <aside className="space-y-4 lg:sticky lg:top-6 lg:self-start">
        <div className="rounded-[20px] border border-border bg-card p-5">
          <h2 className="heading text-lg">{t('promptTitle')}</h2>
          <p className="mt-1 text-sm text-muted-foreground">{t('promptHint')}</p>
          <pre className="mt-3 max-h-56 overflow-auto rounded-[12px] bg-terminal p-3 font-mono text-xs leading-5 whitespace-pre-wrap text-terminal-foreground">{prompt}</pre>
          <div className="mt-3"><CopyButton text={prompt} /></div>
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
    <Button size="lg" onClick={() => void copy()}>
      {copied ? <Check /> : <Copy />}{copied ? t('copied') : t('copy')}
    </Button>
  )
}
