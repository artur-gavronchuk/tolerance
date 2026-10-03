'use client'

import Link from 'next/link'
import { use, useCallback, useEffect, useRef, useState } from 'react'
import { Upload } from 'lucide-react'
import { Markdown } from '@/components/daily/markdown'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { EntryCard } from '@/components/products/entry-card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import { friendlyMessage, products } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { ProductDetail } from '@/lib/types'

const MAX_BYTES = 5 << 20

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

  return (
    <div className="space-y-8">
      <PageHeader
        kicker={<Link href="/products" className="hover:text-foreground">Product tasks</Link>}
        title={task.title}
        actions={<Button variant="outline" render={<Link href={`/products/${slug}/results`} />} nativeButton={false}>Results</Button>}
      >
        <span className="mr-2 inline-block align-middle"><Badge variant={open ? 'default' : 'secondary'}>{open ? 'Open' : 'Voting'}</Badge></span>
        {open ? 'Uploads close' : 'Closed'} {new Date(task.deadline).toLocaleString()}
      </PageHeader>

      <section className="rounded-[14px] border border-border bg-card p-5">
        <Markdown>{task.task_md ?? ''}</Markdown>
      </section>

      <section>
        <SectionTitle aside={open && me && task.attempts < 1000 ? `${left} of ${task.attempts} attempts left` : undefined}>Your entry</SectionTitle>
        {!open && <p className="text-sm text-muted-foreground">The deadline has passed. <Link className="font-semibold text-primary hover:underline" href={`/products/${slug}/results`}>See the results and vote.</Link></p>}
        {open && !meLoading && !me && (
          <p className="text-sm text-muted-foreground"><Link className="font-semibold text-primary hover:underline" href="/login">Sign in</Link> to upload your solution.</p>
        )}
        {open && me && <UploadForm slug={slug} disabled={task.attempts < 1000 && left <= 0} onDone={refresh} />}
        {task.mine.length > 0 && (
          <div className="mt-6 space-y-3">
            {task.mine.map((e) => <EntryCard key={e.id} entry={e} />)}
          </div>
        )}
      </section>
    </div>
  )
}

function UploadForm({ slug, disabled, onDone }: { slug: string; disabled: boolean; onDone: () => void }) {
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
        <Label htmlFor="product-file">Your project</Label>
        <Input id="product-file" ref={fileRef} type="file" accept=".zip" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        <p className="text-xs text-muted-foreground">A .zip with the files at its root, up to 5 MB.</p>
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
