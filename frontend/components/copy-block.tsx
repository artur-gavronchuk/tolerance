'use client'

import { useState } from 'react'
import { Check, Copy } from 'lucide-react'

// Commands and keys, set like a terminal so they read as something to run.
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
    <div className="flex items-start gap-2 rounded-[10px] bg-[#15212b] py-2.5 pr-2 pl-4 text-[#eef2f5] ring-1 ring-white/5">
      <pre className="flex-1 overflow-x-auto py-0.5 font-mono text-[0.8rem] leading-6 whitespace-pre-wrap [overflow-wrap:anywhere]">{text}</pre>
      <button onClick={copy} aria-label={copied ? 'Copied' : 'Copy'}
        className="flex size-8 shrink-0 items-center justify-center rounded-md text-[#eef2f5]/60 hover:bg-white/10 hover:text-[#eef2f5]">
        {copied ? <Check className="size-4 text-[#8fd6b0]" /> : <Copy className="size-4" />}
      </button>
    </div>
  )
}
