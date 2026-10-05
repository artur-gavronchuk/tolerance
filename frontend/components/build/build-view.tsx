'use client'

import { useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { Accessibility, ArrowLeft, Bot, Check, ChevronDown, Copy, Download, Gauge, ListChecks, Smartphone, Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Markdown } from '@/components/daily/markdown'
import { BuildUpload } from '@/components/build/build-upload'
import { BuildGallery } from '@/components/build/build-gallery'
import { api } from '@/lib/api'
import { errorText } from '@/lib/format'
import { KINDS } from '@/lib/build-kinds'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildEntry, BuildPage } from '@/lib/types'
import { cn } from '@/lib/utils'

// What people paste into their agent; agents read English, like TASK.md itself.
const PROMPT = 'Read TASK.md in this folder and build what it describes as static files (index.html at the root, plus any CSS/JS/images; no CDNs, no network). Follow every data-testid in it exactly. When done, zip the folder so index.html is at the root of the zip.'

export function BuildView({ slug }: { slug: string }) {
  const t = useT(buildMessages)
  const [page, setPage] = useState<BuildPage | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setPage(await api<BuildPage>(`/builds/${encodeURIComponent(slug)}`))
      setError(null)
    } catch (e) {
      setError(errorText(e, t.locale))
    }
  }, [slug, t.locale])

  useEffect(() => { void load() }, [load])

  // While the viewer's entry is being scored, poll until it settles.
  const pending = page?.mine?.status === 'queued' || page?.mine?.status === 'running'
  useEffect(() => {
    if (!pending) return
    const id = setInterval(() => void load(), 3000)
    return () => clearInterval(id)
  }, [pending, load])

  const replaceEntry = (e: BuildEntry) => setPage((p) => p && {
    ...p,
    mine: p.mine?.id === e.id ? e : p.mine,
    entries: p.entries.map((x) => (x.id === e.id ? e : x)),
  })

  if (error && !page) return <p role="alert" className="text-destructive">{error}</p>
  if (!page) return <BuildSkeleton />

  const c = page.challenge
  const ru = t.locale === 'ru'
  const title = (ru && c.title_ru) || c.title
  const summary = (ru && c.summary_ru) || c.summary
  const k = KINDS[c.kind]

  return (
    <div className="space-y-10">
      <Link href="/" className="-mb-6 inline-flex items-center gap-1.5 text-sm font-semibold text-muted-foreground hover:text-foreground">
        <ArrowLeft className="size-4" />{t('allChallenges')}
      </Link>
      <section className="relative overflow-hidden rounded-[20px] bg-terminal px-5 py-7 text-terminal-foreground sm:px-9 sm:py-10">
        <div aria-hidden className={cn('pointer-events-none absolute -right-24 -top-24 size-80 rounded-full blur-3xl', k.glow)} />
        <div aria-hidden className="pointer-events-none absolute -bottom-32 left-1/3 size-80 rounded-full bg-pop-1/25 blur-3xl" />
        <k.icon aria-hidden className="pointer-events-none absolute -right-8 top-6 size-56 rotate-12 text-white opacity-[0.06]" />
        <div className="relative">
          <div className="flex flex-wrap items-center gap-2 text-[13px] font-bold">
            <span className={cn('inline-flex items-center gap-1.5 rounded-full bg-gradient-to-r px-3 py-1 text-white', k.gradient)}>
              <k.icon className="size-3.5" />{t(`kind_${c.kind}`)}
            </span>
            <span className="text-terminal-foreground/70">{t.plural('solutions', c.entries)}</span>
          </div>
          <h1 className="display mt-4 max-w-3xl text-[2.6rem] sm:text-[4rem]">
            <span className={cn('bg-gradient-to-r bg-clip-text text-transparent', k.gradient)}>{title}</span>
          </h1>
          <p className="mt-3 max-w-2xl text-[15px] leading-relaxed text-terminal-foreground/80 sm:text-lg">{summary}</p>

          <ol className="mt-7 grid gap-3 md:grid-cols-3">
            <Step n={1} title={t('step1')} text={t('step1Text')}>
              <Button size="lg" className="w-full bg-white text-terminal hover:bg-white/90" nativeButton={false}
                render={<a href={`/api/v1/builds/${encodeURIComponent(c.slug)}/task.zip`} download />}>
                <Download />{t('download')}
              </Button>
            </Step>
            <Step n={2} title={t('step2')} text={t('step2Text')}>
              <CopyPrompt />
            </Step>
            <Step n={3} title={t('step3')} text={t('step3Text')}>
              <Button size="lg" className={cn('w-full border-0 bg-gradient-to-r text-white hover:opacity-90', k.gradient)} nativeButton={false}
                render={<a href="#upload" />}>
                <Upload />{t('toUpload')}
              </Button>
            </Step>
          </ol>
        </div>
      </section>

      <section id="upload" className="grid scroll-mt-6 gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <BuildUpload slug={c.slug} mine={page.mine} onUploaded={(e) => { setPage((p) => p && { ...p, mine: e }) }} />
        <div className="space-y-4">
          <ScoreRules />
          <details className="group rounded-[14px] border border-border bg-card">
            <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-5 py-4 font-bold">
              {t('fullTask')}
              <ChevronDown className="size-4 transition-transform group-open:rotate-180" />
            </summary>
            <div className="border-t border-border px-5 py-4"><Markdown>{c.task_md}</Markdown></div>
          </details>
        </div>
      </section>

      <BuildGallery slug={c.slug} mobile={c.mobile} entries={page.entries} onChange={replaceEntry} />

    </div>
  )
}

function Step({ n, title, text, children }: { n: number; title: string; text: string; children: React.ReactNode }) {
  return (
    <li className="flex flex-col gap-3 rounded-[14px] border border-white/10 bg-white/[0.05] p-4 backdrop-blur-sm">
      <div className="flex items-start gap-3">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-white/10 font-mono text-sm font-bold">{n}</span>
        <div className="min-w-0">
          <p className="font-bold">{title}</p>
          <p className="mt-0.5 text-[13px] leading-snug text-terminal-foreground/70">{text}</p>
        </div>
      </div>
      <div className="mt-auto">{children}</div>
    </li>
  )
}

function CopyPrompt() {
  const t = useT(buildMessages)
  const [copied, setCopied] = useState(false)
  async function copy() {
    try {
      await navigator.clipboard.writeText(PROMPT)
      setCopied(true)
      setTimeout(() => setCopied(false), 1800)
    } catch {}
  }
  return (
    <button type="button" onClick={() => void copy()} title={PROMPT}
      className="flex h-11 w-full items-center justify-center gap-2 rounded-[10px] border border-white/20 bg-white/[0.06] px-4 text-[0.95rem] font-bold transition-colors hover:bg-white/[0.12]">
      {copied ? <Check className="size-4 text-terminal-success" /> : <Copy className="size-4" />}
      {copied ? t('copied') : t('copy')}
    </button>
  )
}

function ScoreRules() {
  const t = useT(buildMessages)
  const rows = [
    { icon: ListChecks, pts: 70, title: t('ptsScenarios'), text: t('ptsScenariosText') },
    { icon: Accessibility, pts: 10, title: t('ptsA11y'), text: t('ptsA11yText') },
    { icon: Smartphone, pts: 10, title: t('ptsMobile'), text: t('ptsMobileText') },
    { icon: Gauge, pts: 10, title: t('ptsPerf'), text: t('ptsPerfText') },
  ]
  return (
    <div className="rounded-[14px] border border-border bg-card p-5">
      <h2 className="heading flex items-center gap-2 text-lg"><Bot className="size-5 text-primary" />{t('howScored')}</h2>
      <ul className="mt-3 space-y-2.5">
        {rows.map((r) => (
          <li key={r.title} className="flex items-start gap-3">
            <span className="w-12 shrink-0 font-mono text-lg font-bold text-primary">{r.pts}</span>
            <span className="min-w-0 text-sm">
              <span className="flex items-center gap-1.5 font-bold"><r.icon className="size-4 text-muted-foreground" />{r.title}</span>
              <span className="text-muted-foreground">{r.text}</span>
            </span>
          </li>
        ))}
      </ul>
      <p className="mt-3 border-t border-border pt-3 text-sm text-muted-foreground">{t('scoreThenVotes')}</p>
    </div>
  )
}

function BuildSkeleton() {
  return (
    <div className="space-y-6">
      <Skeleton className="h-80 w-full rounded-[20px]" />
      <div className="grid gap-6 lg:grid-cols-2">
        <Skeleton className="h-64" />
        <Skeleton className="h-64" />
      </div>
    </div>
  )
}
