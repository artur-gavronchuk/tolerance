'use client'

import { useState } from 'react'
import { Code2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { products } from '@/lib/api'
import { friendly, usePT } from './phase'
import type { ProductSourceFile } from '@/lib/types'

// Reads a published entry's text files in place; nothing is fetched until it is opened.
export function SourceViewer({ id, children }: { id: string; children?: React.ReactNode }) {
  const t = usePT()
  const [files, setFiles] = useState<ProductSourceFile[] | null>(null)
  const [open, setOpen] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function toggle() {
    setOpen(!open)
    if (open || files) return
    try {
      setFiles(await products.source(id))
    } catch (e) {
      setError(friendly(t, e))
    }
  }

  return (
    <div className="min-w-0">
      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="outline" aria-expanded={open} onClick={() => void toggle()}><Code2 />{open ? t('src.hide') : t('src.view')}</Button>
        {children}
      </div>
      {open && (
        <div className="mt-3 min-w-0 space-y-2">
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
          {!files && !error && <p className="text-sm text-muted-foreground">{t('src.loading')}</p>}
          {files?.map((f) => (
            <details key={f.path} open={files.length === 1} className="min-w-0 rounded-[10px] border border-border">
              <summary className="cursor-pointer px-3 py-2 text-sm font-semibold break-all">
                {f.path} <span className="font-normal text-muted-foreground">{f.size} B</span>
              </summary>
              {f.content != null
                ? <pre className="max-h-96 overflow-auto border-t border-border bg-muted/40 p-3 text-xs leading-relaxed whitespace-pre">{f.content}{f.truncated && t('src.truncated')}</pre>
                : <p className="border-t border-border px-3 py-2 text-xs text-muted-foreground">{f.binary ? t('src.binary') : t('src.tooLarge')}</p>}
            </details>
          ))}
        </div>
      )}
    </div>
  )
}
