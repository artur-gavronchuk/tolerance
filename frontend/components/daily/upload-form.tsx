'use client'

import { useRef, useState } from 'react'
import { Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { upload, friendlyMessage } from '@/lib/api'
import type { Submission } from '@/lib/types'

const MAX_BYTES = 5 << 20

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
  const exhausted = attemptsLeft !== undefined && attemptsLeft <= 0

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
        <Label htmlFor="solution-file">Your result</Label>
        <Input id="solution-file" ref={fileRef} type="file" accept=".zip,.patch,.diff"
          onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
        <p className="text-xs text-muted-foreground">
          A .zip of the edited repository (files at the root) or a .patch / .diff, up to 5 MB.
        </p>
      </div>
      <div className="space-y-2">
        <Label htmlFor="made-with">Made with <span className="font-normal text-muted-foreground">(optional)</span></Label>
        <Input id="made-with" value={madeWith} maxLength={100} placeholder="Claude Code + Opus"
          onChange={(e) => setMadeWith(e.target.value)} />
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
