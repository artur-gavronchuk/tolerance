'use client'

import { useRef, useState } from 'react'
import { Upload } from 'lucide-react'
import { PageHeader } from '@/components/page-header'
import { ReplayPlayer } from '@/components/tanks/viewer/replay-player'
import { Button } from '@/components/ui/button'
import { CLI } from '@/lib/brand'
import { cn } from '@/lib/utils'
import { parseReplayFile, type Replay } from '@/lib/tanks/replay'
import { useT } from '@/lib/i18n/client'
import { tanksMatchMessages } from '@/lib/i18n/messages/tanks-match'

export default function ReplayFilePage() {
  const t = useT(tanksMatchMessages)
  const [replay, setReplay] = useState<Replay | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [dragging, setDragging] = useState(false)
  const inputRef = useRef<HTMLInputElement | null>(null)

  async function open(file: File) {
    try {
      const r = await parseReplayFile(file)
      setReplay(r)
      setError(null)
    } catch {
      setError(t('fileReadError', { cli: CLI }))
    }
  }

  return (
    <div className="mx-auto max-w-5xl px-4 py-10 sm:px-6 sm:py-14">
      <PageHeader title={t('openReplay')}>
        {t('openReplayLead')} <code>{CLI} tanks play</code>.
      </PageHeader>

      {!replay && (
        <div
          onDragOver={(e) => { e.preventDefault(); setDragging(true) }}
          onDragLeave={() => setDragging(false)}
          onDrop={(e) => {
            e.preventDefault()
            setDragging(false)
            const f = e.dataTransfer.files[0]
            if (f) void open(f)
          }}
          className={cn(
            'mt-8 flex flex-col items-center gap-4 rounded-2xl border-2 border-dashed px-6 py-16 text-center transition-colors',
            dragging ? 'border-primary bg-accent' : 'border-input'
          )}
        >
          <Upload className="size-8 text-muted-foreground" />
          <div>
            <p className="font-semibold">{t('dropHere')}</p>
            <p className="mt-1 text-sm text-muted-foreground">
              {t('orChoose')} <code>tanks-replay.json</code> {t('orChooseMid')} <code>.json.gz</code> {t('orChooseEnd')}
            </p>
          </div>
          <Button onClick={() => inputRef.current?.click()}>{t('chooseFile')}</Button>
          <input
            ref={inputRef}
            type="file"
            accept=".json,.gz"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0]
              if (f) void open(f)
            }}
          />
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        </div>
      )}

      {replay && (
        <div className="mt-8 space-y-4">
          <Button variant="outline" onClick={() => setReplay(null)}>{t('openAnother')}</Button>
          <ReplayPlayer replay={replay} />
        </div>
      )}
    </div>
  )
}
