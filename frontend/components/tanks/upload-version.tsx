'use client'

import { useRef, useState } from 'react'
import { Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { CopyBlock } from '@/components/copy-block'
import { post, friendlyMessage } from '@/lib/api'
import { CLI } from '@/lib/brand'
import type { VersionView } from '@/lib/types'

// The package limit (spec): at most 1 MiB compressed. Checked client-side so
// an oversized file never even gets base64-encoded and sent.
const MAX_ARCHIVE_BYTES = 1 << 20

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

// The "write it by hand, upload the archive" path: no connector needed. It
// goes through the same botpkg.Normalize checks (single root folder and
// macOS junk stripped, language/entry inferred) as `arena tanks submit`.
export function UploadVersion({ onUploaded }: { onUploaded: (v: VersionView) => void }) {
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function onChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    setError(null)
    if (file.size > MAX_ARCHIVE_BYTES) {
      setError(`That archive is ${(file.size / (1 << 20)).toFixed(2)} MiB compressed; the limit is 1 MiB.`)
      return
    }
    setBusy(true)
    try {
      const archive_base64 = await fileToBase64(file)
      const v = await post<VersionView>('/me/tanks/versions', { archive_base64 })
      onUploaded(v)
    } catch (err) {
      setError(friendlyMessage(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <Button variant="outline" type="button" disabled={busy} onClick={() => inputRef.current?.click()}>
          <Upload />{busy ? 'Uploading…' : 'Upload by hand'}
        </Button>
        <input ref={inputRef} type="file" accept=".tar.gz,.tgz,.gz" className="hidden" onChange={(e) => void onChange(e)} />
        <span className="text-xs text-muted-foreground">.tar.gz or .tgz, up to 1 MiB compressed</span>
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      <CopyBlock text={`tar czf bot.tar.gz -C mybot .\n${CLI} tanks submit mybot`} />
      <p className="text-xs text-muted-foreground">
        Either pack your bot&apos;s folder yourself and drop the archive above, or run <code>{CLI} tanks submit</code>{' '}
        from the connector once you&apos;re logged in — same checks, no browser upload needed.
      </p>
    </div>
  )
}
