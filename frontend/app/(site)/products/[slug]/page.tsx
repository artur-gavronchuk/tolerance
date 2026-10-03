'use client'

import Link from 'next/link'
import { use, useCallback, useEffect, useRef, useState } from 'react'
import { Upload } from 'lucide-react'
import { Markdown } from '@/components/daily/markdown'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { AdminBar } from '@/components/products/admin-bar'
import { EntryCard } from '@/components/products/entry-card'
import { EntryGallery } from '@/components/products/gallery'
import { PhaseBadge, VotingNote, rankRuleText } from '@/components/products/phase'
import { useResults } from '@/components/products/use-results'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { friendlyMessage, products } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { ProductDetail, ProductEntry } from '@/lib/types'

const MAX_BYTES = 5 << 20

// The upload of a person that stands for them in the results (see the ranking rule on the page).
function countedId(task: ProductDetail): string | null {
  const done = task.mine.filter((e) => e.status === 'done') // newest first
  if (done.length === 0) return null
  if (task.kind === 'site') return done[0].id
  return done.reduce((best, e) => (e.passed >= best.passed ? e : best)).id
}

export default function ProductPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = use(params)
  const { me, loading: meLoading } = useMe()
  const [task, setTask] = useState<ProductDetail | null>(null)
  const [error, setError] = useState<string | null>(null)
  const refresh = useCallback(() => {
    products.get(slug).then(setTask).catch((e) => setError(friendlyMessage(e)))
  }, [slug])
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
        kicker={<Link href="/products" className="hover:text-foreground">Product tasks</Link>}
        title={task.title}
        actions={<Button variant="outline" render={<Link href={`/products/${slug}/results`} />} nativeButton={false}>Results</Button>}
      >
        <span className="mr-2 inline-block align-middle"><PhaseBadge phase={task.phase} /></span>
        {open ? 'Uploads close' : 'Uploads closed'} {new Date(task.deadline).toLocaleString('en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })}
        {open && <> · {task.entry_count} {task.entry_count === 1 ? 'entry' : 'entries'} submitted so far</>}
      </PageHeader>

      {me?.can_admin && <AdminBar task={task} onChange={refresh} />}

      {!open && <Published task={task} slug={slug} viewer={me?.user.handle} signedIn={!!me} />}

      {open ? (
        <section className="rounded-[14px] border border-border bg-card p-5">
          <Markdown>{task.task_md ?? ''}</Markdown>
        </section>
      ) : (
        <details className="rounded-[14px] border border-border bg-card p-5">
          <summary className="heading cursor-pointer text-lg">The task</summary>
          <div className="mt-3"><Markdown>{task.task_md ?? ''}</Markdown></div>
        </details>
      )}

      <section>
        <SectionTitle aside={open && me && task.attempts < 1000 ? `${left} of ${task.attempts} attempts left` : undefined}>
          {open ? 'Your entry' : 'Your uploads'}
        </SectionTitle>
        {open && (
          <p className="mb-4 max-w-2xl text-sm text-muted-foreground">
            Entries stay hidden until the deadline. {rankRuleText(task.kind, task.scenario_count > 0)}
          </p>
        )}
        {open && !meLoading && !me && (
          <p className="text-sm text-muted-foreground"><Link className="font-semibold text-primary hover:underline" href="/login">Sign in</Link> to upload your solution.</p>
        )}
        {open && me && <UploadForm slug={slug} site={task.kind === 'site'} disabled={task.attempts < 1000 && left <= 0} onDone={refresh} />}
        {!open && task.mine.length === 0 && (
          <p className="text-sm text-muted-foreground">{me ? 'You did not upload anything for this task.' : 'Sign in to see your own uploads.'}</p>
        )}
        {task.mine.length > 0 && (
          <div className="mt-6 space-y-3">
            {task.mine.map((e: ProductEntry) => <EntryCard key={e.id} entry={e} site={task.kind === 'site'} counts={e.id === counted} preview={open} />)}
          </div>
        )}
      </section>
    </div>
  )
}

// After the deadline: the standings note, the voting window and the gallery of everyone's entries.
function Published({ task, slug, viewer, signedIn }: { task: ProductDetail; slug: string; viewer: string | undefined; signedIn: boolean }) {
  const { res, error, voteError, busy, toggleVote } = useResults(slug, viewer, true)
  return (
    <section className="space-y-4">
      <SectionTitle aside={<Link className="font-semibold text-primary hover:underline" href={`/products/${slug}/results`}>Podium and table</Link>}>
        Entries{res && ` (${res.entries.length})`}
      </SectionTitle>
      <VotingNote task={task} />
      <p className="max-w-2xl text-sm text-muted-foreground">{rankRuleText(task.kind, task.scenario_count > 0)}</p>
      {voteError && <p role="alert" className="text-sm text-destructive">{voteError}</p>}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {!res && !error && <Skeleton className="h-40 rounded-[14px]" />}
      {res && res.entries.length === 0 && (
        <p className="rounded-[14px] border border-dashed border-input px-5 py-10 text-center text-sm text-muted-foreground">Nobody entered this task.</p>
      )}
      {res && res.entries.length > 0 && (
        <EntryGallery task={res.task} entries={res.entries} signedIn={signedIn} busy={busy} onToggleVote={(e) => void toggleVote(e)} />
      )}
    </section>
  )
}

function UploadForm({ slug, site, disabled, onDone }: { slug: string; site: boolean; disabled: boolean; onDone: () => void }) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [madeWith, setMadeWith] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!file) return
    if (file.size > MAX_BYTES) {
      setError('That file is over 5 MB.')
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
      setError(friendlyMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={(e) => void submit(e)} className="max-w-xl space-y-4">
      <div className="space-y-2">
        <Label htmlFor="product-file">{site ? 'Your site' : 'Your project'}</Label>
        <Input id="product-file" ref={fileRef} type="file" accept=".zip" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        <p className="text-xs text-muted-foreground">
          {site ? 'A .zip of the static site with index.html at its root, up to 5 MB. Use relative paths for assets.' : 'A .zip with the files at its root, up to 5 MB.'}
        </p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="product-made-with">Made with <span className="font-normal text-muted-foreground">(optional)</span></Label>
        <Input id="product-made-with" value={madeWith} maxLength={100} placeholder="Claude Code + Opus" onChange={(e) => setMadeWith(e.target.value)} />
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <Button type="submit" disabled={busy || !file || disabled}><Upload />{busy ? 'Uploading…' : 'Submit'}</Button>
    </form>
  )
}
