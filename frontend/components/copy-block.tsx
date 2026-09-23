'use client'

import { useState } from 'react'
import { Check, Copy } from 'lucide-react'

export function CopyBlock({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)
  async function copy() {
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {}
  }
  return (
    <div className="flex items-start gap-2 rounded-md border border-border bg-muted/40 px-3 py-2">
      <pre className="flex-1 overflow-x-auto whitespace-pre-wrap break-all font-mono text-xs">{text}</pre>
      <button onClick={copy} aria-label="Copy" className="shrink-0 rounded p-1 text-muted-foreground hover:text-foreground">
        {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
      </button>
    </div>
  )
}
