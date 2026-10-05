'use client'

import { useRef, useState } from 'react'
import Link from 'next/link'
import { AlertTriangle, ExternalLink, FileCheck, Loader2, Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { useLoginHref } from '@/components/public/return-path'
import { ScoreBreakdown, ScoreBadge, entryHref, shotUrl, siteUrl } from '@/components/build/build-gallery'
import { upload } from '@/lib/api'
import { errorText } from '@/lib/format'
import { formatNumber } from '@/lib/i18n/core'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildEntry } from '@/lib/types'
import { useMe } from '@/lib/use-me'
import { cn } from '@/lib/utils'

const MAX_BYTES = 5 << 20

export function BuildUpload({ slug, mine, onUploaded }: { slug: string; mine: BuildEntry | null; onUploaded: (e: BuildEntry) => void }) {
  const t = useT(buildMessages)
  const { me, loading } = useMe()
  const loginHref = useLoginHref()
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [madeWith, setMadeWith] = useState(mine?.made_with ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [dragging, setDragging] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!file) return
    if (file.size > MAX_BYTES) {
      setError(t('over5'))
      return
    }
    setBusy(true)
    setError(null)
    try {
      const form = new FormData()
      form.set('file', file)
      if (madeWith.trim()) form.set('made_with', madeWith.trim())
      onUploaded(await upload<BuildEntry>(`/builds/${encodeURIComponent(slug)}/entries`, form))
      setFile(null)
      if (fileRef.current) fileRef.current.value = ''
    } catch (err) {
      setError(errorText(err, t.locale))
    } finally {
      setBusy(false)
    }
  }

  const sizeText = (n: number) => n < 1 << 20
    ? `${formatNumber(t.locale, n / 1024, { maximumFractionDigits: 1 })} ${t.locale === 'ru' ? 'КБ' : 'KB'}`
    : `${formatNumber(t.locale, n / (1 << 20), { maximumFractionDigits: 2 })} ${t.locale === 'ru' ? 'МБ' : 'MB'}`

  return (
    <div className="rounded-[14px] border border-border bg-card p-5">
      <h2 className="heading text-lg">{t('uploadTitle')}</h2>
      {mine && <MyEntry slug={slug} e={mine} />}
      {loading ? null : !me ? (
        <div className="mt-4 rounded-[12px] border-2 border-dashed border-strong px-4 py-8 text-center">
          <p className="text-sm text-muted-foreground">{t('signInToUpload')}</p>
          <Button className="mt-3" nativeButton={false} render={<Link href={loginHref} />}>{t('signIn')}</Button>
        </div>
      ) : (
        <form onSubmit={(e) => void submit(e)} className="mt-4 space-y-4">
          <button type="button" disabled={busy} onClick={() => fileRef.current?.click()}
            onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
            onDragLeave={() => setDragging(false)}
            onDrop={(e) => {
              e.preventDefault()
              setDragging(false)
              const f = e.dataTransfer.files?.[0]
              if (f && !busy) { setFile(f); setError(null) }
            }}
            className={cn('flex w-full flex-col items-center gap-1.5 rounded-[12px] border-2 border-dashed px-4 py-8 text-center transition-colors disabled:opacity-60',
              dragging ? 'border-primary bg-primary/5' : 'border-input hover:bg-muted/50')}>
            {file ? <FileCheck className="size-6 text-success" /> : <Upload className="size-6 text-muted-foreground" />}
            {file ? (
              <>
                <span className="max-w-full truncate font-mono text-sm font-bold" title={file.name}>{file.name}</span>
                <span className="text-xs text-muted-foreground">{t('replaceHint', { size: sizeText(file.size) })}</span>
              </>
            ) : (
              <>
                <span className="font-bold">{t('dropTitle')}</span>
                <span className="text-xs text-muted-foreground">{t('dropHint')}</span>
              </>
            )}
          </button>
          <input ref={fileRef} type="file" accept=".zip,.html,.htm" className="hidden" aria-label={t('dropTitle')}
            onChange={(e) => { const f = e.target.files?.[0]; if (f) { setFile(f); setError(null) } }} />
          <div className="space-y-2">
            <Label htmlFor="build-made-with">{t('madeWith')} <span className="font-normal text-muted-foreground">{t('optional')}</span></Label>
            <Input id="build-made-with" value={madeWith} maxLength={100} placeholder="Claude Code + Opus" list="build-made-with-list"
              onChange={(e) => setMadeWith(e.target.value)} />
            <datalist id="build-made-with-list">
              {['Claude Code + Opus', 'Claude Code + Sonnet', 'Codex CLI + GPT-5', 'Cursor', 'Gemini CLI', 'Windsurf', 'Lovable', 'v0', 'Bolt'].map((o) => <option key={o} value={o} />)}
            </datalist>
          </div>
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
          <Button type="submit" size="lg" disabled={busy || !file} className="w-full sm:w-auto">
            <Upload />{busy ? t('uploading') : mine ? t('reupload') : t('submit')}
          </Button>
        </form>
      )}
    </div>
  )
}

function MyEntry({ slug, e }: { slug: string; e: BuildEntry }) {
  const t = useT(buildMessages)
  if (e.status === 'queued' || e.status === 'running') {
    return (
      <div className="mt-4 flex items-start gap-3 rounded-[12px] bg-accent p-4 text-accent-foreground">
        <Loader2 className="mt-0.5 size-5 shrink-0 animate-spin" />
        <div>
          <p className="font-bold">{e.status === 'queued' ? t('queued') : t('running')}</p>
          <p className="mt-0.5 text-sm opacity-80">{t('runningHint')}</p>
        </div>
      </div>
    )
  }
  if (e.status === 'infra_error') {
    return (
      <div className="mt-4 flex items-start gap-3 rounded-[12px] bg-destructive/10 p-4 text-destructive">
        <AlertTriangle className="mt-0.5 size-5 shrink-0" />
        <p className="text-sm font-semibold">{t('infraError')}</p>
      </div>
    )
  }
  return (
    <div className="mt-4 overflow-hidden rounded-[12px] border border-border">
      <Link href={entryHref(slug, e)} className="group relative block aspect-[16/10] bg-muted">
        {e.has_shot
          // eslint-disable-next-line @next/next/no-img-element
          ? <img src={shotUrl(e)} alt="" className="size-full object-cover object-top" />
          : <iframe src={siteUrl(e)} title={e.handle} sandbox="allow-scripts" loading="lazy" tabIndex={-1}
              className="pointer-events-none h-[250%] w-[250%] origin-top-left scale-[0.4] border-0 bg-white" />}
        <span className="absolute right-2 top-2 flex items-center gap-1 rounded-full bg-terminal/80 px-2.5 py-1 text-xs font-bold text-terminal-foreground opacity-0 transition-opacity group-hover:opacity-100">
          <ExternalLink className="size-3" />{t('open')}
        </span>
      </Link>
      <div className="space-y-3 p-4">
        <div className="flex items-center justify-between gap-3">
          <span className="text-sm font-bold text-muted-foreground">{t('yourScore')}</span>
          <ScoreBadge score={e.score} large />
        </div>
        {e.failure_reason === 'timeout' && <p className="text-sm text-warning">{t('timeout')}</p>}
        {e.failure_reason === 'no_results' && <p className="text-sm text-warning">{t('noResults')}</p>}
        <ScoreBreakdown e={e} />
        {(e.checks.failed?.length ?? 0) > 0 && (
          <details className="text-sm">
            <summary className="cursor-pointer font-semibold">{t('failedScenarios')} ({e.checks.failed!.length})</summary>
            <ul className="mt-2 list-disc space-y-0.5 pl-5 text-muted-foreground">
              {e.checks.failed!.map((n) => <li key={n}>{n}</li>)}
            </ul>
          </details>
        )}
        {e.log_tail && (
          <details className="text-sm">
            <summary className="cursor-pointer font-semibold">{t('log')}</summary>
            <pre className="mt-2 max-h-64 overflow-auto rounded-[8px] bg-terminal p-3 font-mono text-xs leading-5 whitespace-pre-wrap text-terminal-foreground">{e.log_tail}</pre>
          </details>
        )}
      </div>
    </div>
  )
}
