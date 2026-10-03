'use client'

import { useRef, useState } from 'react'
import { FileCheck, Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { upload, friendlyMessage } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { Submission } from '@/lib/types'

const MAX_BYTES = 5 << 20

const fmtSize = (n: number) => (n < 1024 ? `${n} B` : n < 1 << 20 ? `${(n / 1024).toFixed(1)} KB` : `${(n / (1 << 20)).toFixed(2)} MB`)

// taskSlug is sent only for practice uploads (a past day); for today's task
// the backend defaults to it.
export function UploadForm({ taskSlug, attemptsLeft, onSubmitted }: {
  taskSlug?: string
  attemptsLeft?: number
  onSubmitted: (s: Submission) => void
}) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [file, setFile] = useState<File | null>(null)
  const [madeWith, setMadeWith] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [dragging, setDragging] = useState(false)
  const exhausted = attemptsLeft !== undefined && attemptsLeft <= 0

  function pick(f: File) {
    setFile(f)
    setError(null)
  }

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
      if (taskSlug) form.set('task_slug', taskSlug)
      const s = await upload<Submission>('/submissions', form)
      onSubmitted(s)
      setFile(null)
      if (fileRef.current) fileRef.current.value = ''
    } catch (err) {
      setError(friendlyMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={(e) => void submit(e)} className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="solution-file">Your solution (.zip or .patch)</Label>
        <button
          type="button"
          disabled={busy}
          onClick={() => fileRef.current?.click()}
          onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault()
            setDragging(false)
            if (busy) return
            const f = e.dataTransfer.files?.[0]
            if (f) pick(f)
          }}
          className={cn(
            'flex w-full flex-col items-center gap-1.5 rounded-[12px] border-2 border-dashed px-4 py-6 text-center transition-colors disabled:opacity-60',
            dragging ? 'border-primary bg-primary/5' : 'border-input hover:bg-muted/50',
          )}>
          {file ? <FileCheck className="size-5 text-success" /> : <Upload className="size-5 text-muted-foreground" />}
          {file ? (
            <>
              <span className="max-w-full truncate font-mono text-sm font-bold" title={file.name}>{file.name}</span>
              <span className="text-xs text-muted-foreground">{fmtSize(file.size)} · click or drop to replace</span>
            </>
          ) : (
            <>
              <span className="text-sm font-bold">Drop your file here, or click to choose</span>
              <span className="text-xs text-muted-foreground">.zip of the edited repository, or a .patch / .diff, up to 5 MB</span>
            </>
          )}
        </button>
        <input id="solution-file" ref={fileRef} type="file" accept=".zip,.patch,.diff" className="hidden"
          onChange={(e) => { const f = e.target.files?.[0]; if (f) pick(f) }} />
      </div>
      <div className="space-y-2">
        <Label htmlFor="made-with">Made with <span className="font-normal text-muted-foreground">(optional)</span></Label>
        <Input id="made-with" value={madeWith} maxLength={100} placeholder="Claude Code + Opus"
          list="made-with-suggestions" onChange={(e) => setMadeWith(e.target.value)} />
        <datalist id="made-with-suggestions">
          {['Claude Code + Opus', 'Claude Code + Sonnet', 'Codex CLI + GPT-5', 'Cursor + Claude Sonnet', 'Cursor + GPT-5', 'Aider + DeepSeek', 'Gemini CLI', 'Cline + Claude Sonnet', 'GitHub Copilot', 'Windsurf', 'OpenHands', 'Custom agent'].map((o) => <option key={o} value={o} />)}
        </datalist>
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <div className="flex flex-wrap items-center gap-3">
        <Button type="submit" disabled={busy || !file || exhausted}>
          <Upload />{busy ? 'Uploading…' : 'Submit'}
        </Button>
        {attemptsLeft !== undefined && (
          <span className="text-sm text-muted-foreground">
            {exhausted ? 'No attempts left today.' : `${attemptsLeft} attempt${attemptsLeft === 1 ? '' : 's'} left today`}
          </span>
        )}
      </div>
    </form>
  )
}
