'use client'

import { useEffect, useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { SectionTitle } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { useT } from '@/lib/i18n/client'
import { badgesMessages as m } from '@/lib/i18n/messages/badges'

// Preview of a public badge plus Markdown/HTML snippets with absolute URLs (the origin the page is served from).
export function ReadmeBadge({ kind, id, href }: { kind: 'u' | 'bot'; id: string; href: string }) {
  const t = useT(m)
  const [origin, setOrigin] = useState('')
  const [copied, setCopied] = useState<'md' | 'html' | null>(null)
  useEffect(() => setOrigin(window.location.origin), [])

  const img = `${origin}/api/v1/badges/${kind}/${encodeURIComponent(id)}.svg`
  const page = `${origin}${href}`
  const alt = t('alt')
  const snippets = {
    md: `[![${alt}](${img})](${page})`,
    html: `<a href="${page}"><img src="${img}" alt="${alt}"></a>`,
  }
  async function copy(k: 'md' | 'html') {
    try {
      await navigator.clipboard.writeText(snippets[k])
      setCopied(k)
      setTimeout(() => setCopied(null), 1500)
    } catch {}
  }

  return (
    <section>
      <SectionTitle>{t('title')}</SectionTitle>
      <div className="space-y-3 rounded-[14px] border border-border bg-card p-4 sm:p-5">
        <p className="text-sm text-muted-foreground">{t('intro')}</p>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src={`/api/v1/badges/${kind}/${encodeURIComponent(id)}.svg`} alt={alt} height={20} className="h-5" />
        <div className="flex flex-wrap gap-2">
          {(['md', 'html'] as const).map((k) => (
            <Button key={k} size="sm" variant="outline" onClick={() => copy(k)}>
              {copied === k ? <Check /> : <Copy />}
              {copied === k ? t('copied') : `${t('copy')} ${k === 'md' ? t('markdown') : t('html')}`}
            </Button>
          ))}
        </div>
      </div>
    </section>
  )
}
