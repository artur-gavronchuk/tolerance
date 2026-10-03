'use client'

import Link from 'next/link'
import { use, useCallback, useEffect, useRef, useState } from 'react'
import { Upload } from 'lucide-react'
import { Markdown } from '@/components/daily/markdown'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { AdminBar } from '@/components/products/admin-bar'
import { Compare } from '@/components/products/compare'
import { EntryCard } from '@/components/products/entry-card'
import { EntryGallery } from '@/components/products/gallery'
import { PhaseBadge, RankingHow, VotingNote, friendly, isBlind, usePT, utc } from '@/components/products/phase'
import { useResults } from '@/components/products/use-results'
import { useLoginHref } from '@/components/public/return-path'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { products } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { ProductDetail, ProductEntry } from '@/lib/types'

const MAX_BYTES = 5 << 20

// The upload of a person that stands for them in the results (see the ranking rule on the page).
function countedId(task: ProductDetail): string | null {
  const done = task.mine.filter((e) => e.status === 'done') // newest first
  if (done.length === 0) return null
  if (task.kind === 'site') return done[0].id
  // the latest upload with the most scenarios passed (the list is newest first, so the first of the best wins)
  return done.reduce((best, e) => (e.passed > best.passed ? e : best)).id
}

export default function ProductPage({ params }: { params: Promise<{ slug: string }> }) {
  const t = usePT()
  const loginTo = useLoginHref()
  const { slug } = use(params)
  const { me, loading: meLoading } = useMe()
  const [task, setTask] = useState<ProductDetail | null>(null)
  const [error, setError] = useState<string | null>(null)
  const refresh = useCallback(() => {
    products.get(slug).then(setTask).catch((e) => setError(friendly(t, e)))
  }, [slug, t])
  useEffect(() => { refresh() }, [refresh, me?.user.handle])

  const pending = task?.mine.some((e) => e.status === 'queued' || e.status === 'running') ?? false
  useEffect(() => {
    if (!pending) return
    const t = setInterval(refresh, 2000)
    return () => clearInterval(t)
  }, [pending, refresh])

  if (error) return <p role="alert" className="text-sm text-destructive">{error}</p>
  if (!task) return <Skeleton className="h-64 rounded-[14px]" />
  const open = task.phase === 'open'
  const left = task.attempts - task.attempts_used
  const counted = countedId(task)

  return (
    <div className="space-y-8">
      <PageHeader
        kicker={<Link href="/products" className="hover:text-foreground">{t('list.title')}</Link>}
        title={task.title}
        actions={!isBlind(task) && <Button variant="outline" render={<Link href={`/products/${slug}/results`} />} nativeButton={false}>{t('task.results')}</Button>}
      >
        <span className="mr-2 inline-block align-middle"><PhaseBadge phase={task.phase} /></span>
        {open ? t('task.uploadsClose') : t('task.uploadsClosed')} {utc(t.locale, task.deadline)}
        {open && t('task.submitted', { count: t.plural('entries', task.entry_count) })}
      </PageHeader>

      {me?.can_admin && <AdminBar task={task} onChange={refresh} />}

      {!open && <Published task={task} slug={slug} viewer={me?.user.handle} signedIn={!!me} />}

      {open ? (
        <section className="rounded-[14px] border border-border bg-card p-5">
          <Markdown>{task.task_md ?? ''}</Markdown>
        </section>
      ) : (
        <details className="rounded-[14px] border border-border bg-card p-5">
          <summary className="heading cursor-pointer text-lg">{t('task.theTask')}</summary>
          <div className="mt-3"><Markdown>{task.task_md ?? ''}</Markdown></div>
        </details>
      )}

      <section>
        <SectionTitle aside={open && me && task.attempts < 1000 ? t('task.attemptsLeft', { left, total: task.attempts }) : undefined}>
          {open ? t('task.yourEntry') : t('task.yourUploads')}
        </SectionTitle>
        {open && (
          <div className="mb-4 space-y-3">
            <p className="max-w-2xl text-sm text-muted-foreground">{t('task.hidden')}</p>
            <RankingHow kind={task.kind} hasChecks={task.scenario_count > 0} />
          </div>
        )}
        {open && !meLoading && !me && (
          <p className="text-sm text-muted-foreground"><Link className="font-semibold text-primary hover:underline" href={loginTo}>{t('task.signInLink')}</Link>{t('task.signInUpload')}</p>
        )}
        {open && me && <UploadForm slug={slug} site={task.kind === 'site'} disabled={task.attempts < 1000 && left <= 0} onDone={refresh} />}
        {!open && task.mine.length === 0 && (
          <p className="text-sm text-muted-foreground">{me ? t('task.noUploads') : t('task.signInSee')}</p>
        )}
        {task.mine.length > 0 && (
          <div className="mt-6 space-y-3">
            {task.mine.map((e: ProductEntry) => <EntryCard key={e.id} entry={e} site={task.kind === 'site'} counts={e.id === counted} preview={open} bench={task.has_bench} fastest={task.fastest_ms} />)}
          </div>
        )}
      </section>
    </div>
  )
}

// After the deadline: the standings note, the voting window and the gallery of everyone's entries.
function Published({ task, slug, viewer, signedIn }: { task: ProductDetail; slug: string; viewer: string | undefined; signedIn: boolean }) {
  const t = usePT()
  const { res, error, voteError, busy, toggleVote } = useResults(slug, viewer, true)
  return (
    <section className="space-y-4">
      <SectionTitle aside={isBlind(task) ? undefined : <Link className="font-semibold text-primary hover:underline" href={`/products/${slug}/results`}>{t('task.podiumTable')}</Link>}>
        {t('task.entries')}{res && ` (${res.entries.length})`}
      </SectionTitle>
      <VotingNote task={task} />
      {task.kind === 'site' && task.phase === 'voting' && <Compare slug={slug} signedIn={signedIn} />}
      <RankingHow kind={task.kind} hasChecks={task.scenario_count > 0 && !isBlind(task)} />
      {voteError && <p role="alert" className="text-sm text-destructive">{voteError}</p>}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!res && !error && <Skeleton className="h-40 rounded-[14px]" />}
      {res && res.entries.length === 0 && (
        <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">{t('task.nobody')}</p>
      )}
      {res && res.entries.length > 0 && (
        <EntryGallery task={res.task} entries={res.entries} signedIn={signedIn} busy={busy} onToggleVote={(e) => void toggleVote(e)} />
      )}
    </section>
  )
}

function UploadForm({ slug, site, disabled, onDone }: { slug: string; site: boolean; disabled: boolean; onDone: () => void }) {
  const t = usePT()
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [madeWith, setMadeWith] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!file) return
    if (file.size > MAX_BYTES) {
      setError(t('up.over5'))
      return
    }
    setBusy(true)
    setError(null)
    try {
      const form = new FormData()
      form.set('file', file)
      if (madeWith.trim()) form.set('made_with', madeWith.trim())
      await products.submit(slug, form)
      setFile(null)
      if (fileRef.current) fileRef.current.value = ''
      onDone()
    } catch (err) {
      setError(friendly(t, err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={(e) => void submit(e)} className="max-w-xl space-y-4">
      <div className="space-y-2">
        <Label htmlFor="product-file">{site ? t('up.yourSite') : t('up.yourProject')}</Label>
        <Input id="product-file" ref={fileRef} type="file" accept=".zip" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        <p className="text-xs text-muted-foreground">
          {site ? t('up.helpSite') : t('up.helpCli')}
        </p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="product-made-with">{t('up.madeWith')} <span className="font-normal text-muted-foreground">{t('up.optional')}</span></Label>
        <Input id="product-made-with" value={madeWith} maxLength={100} placeholder="Claude Code + Opus" onChange={(e) => setMadeWith(e.target.value)} />
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <Button type="submit" disabled={busy || !file || disabled}><Upload />{busy ? t('up.uploading') : t('up.submit')}</Button>
    </form>
  )
}
