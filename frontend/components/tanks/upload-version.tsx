'use client'

import { useRef, useState } from 'react'
import { Upload } from 'lucide-react'
import { post, friendlyMessage } from '@/lib/api'
import type { VersionView } from '@/lib/types'
import { cn } from '@/lib/utils'

// The package limit: at most 1 MiB compressed. Checked client-side so an
// oversized file never even gets base64-encoded and sent.
const MAX_ARCHIVE_BYTES = 1 << 20

const ACCEPTED = /\.(zip|tar\.gz|tgz)$/i

// Reads a File into a base64 string without going through FileReader's
// data-URL form (which would need stripping the "data:...;base64," prefix).
// Chunked so a ~1 MiB file doesn't blow the argument limit on
// String.fromCharCode(...bytes).
async function fileToBase64(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer())
  let binary = ''
  const chunkSize = 0x8000
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize))
  }
  return btoa(binary)
}

// Drop zone for a bot folder zipped (or tarred) by the owner. The server runs
// the same botpkg.Normalize checks for both formats: a single wrapping folder
// and macOS junk are stripped, the language and entry come from bot.json.
export function UploadVersion({ onUploaded }: { onUploaded: (v: VersionView) => void }) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [over, setOver] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [done, setDone] = useState<VersionView | null>(null)

  async function send(file: File) {
    setError(null)
    setDone(null)
    if (!ACCEPTED.test(file.name)) {
      setError('Upload a .zip, .tar.gz or .tgz file.')
      return
    }
    if (file.size > MAX_ARCHIVE_BYTES) {
      setError(`That archive is ${(file.size / (1 << 20)).toFixed(2)} MiB; the limit is 1 MiB.`)
      return
    }
    setBusy(true)
    try {
      const archive_base64 = await fileToBase64(file)
      const v = await post<VersionView>('/me/tanks/versions', { archive_base64 })
      setDone(v)
      onUploaded(v)
    } catch (err) {
      setError(friendlyMessage(err))
    } finally {
      setBusy(false)
    }
  }

  function onChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (file) void send(file)
  }

  function onDrop(e: React.DragEvent) {
    e.preventDefault()
    setOver(false)
    if (busy) return
    const file = e.dataTransfer.files?.[0]
    if (file) void send(file)
  }

  return (
    <div className="space-y-3">
      <button
        type="button"
        disabled={busy}
        onClick={() => inputRef.current?.click()}
        onDragOver={(e) => { e.preventDefault(); setOver(true) }}
        onDragLeave={() => setOver(false)}
        onDrop={onDrop}
        className={cn(
          'flex w-full flex-col items-center gap-1.5 rounded-[12px] border-2 border-dashed px-4 py-8 text-center transition-colors disabled:opacity-60',
          over ? 'border-primary bg-primary/5' : 'border-input hover:bg-muted/50',
        )}
      >
        <Upload className="size-5 text-muted-foreground" />
        <span className="text-sm font-bold">{busy ? 'Uploading…' : 'Drop your bot .zip here, or click to choose'}</span>
        <span className="text-xs text-muted-foreground">.zip, .tar.gz or .tgz, up to 1 MiB</span>
      </button>
      <input ref={inputRef} type="file" accept=".zip,.tar.gz,.tgz,.gz" className="hidden" onChange={onChange} />
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {done && (
        <p role="status" className="text-sm text-muted-foreground">
          Version {done.number} uploaded. The platform is playing a trial match for it now; progress and the replay
          appear under Versions below.
        </p>
      )}
    </div>
  )
}
