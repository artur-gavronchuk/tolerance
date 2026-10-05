'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowRight } from 'lucide-react'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildPage } from '@/lib/types'

// The home page's way into the build challenge: its current task, one click away.
export function BuildBanner() {
  const t = useT(buildMessages)
  const [page, setPage] = useState<BuildPage | null>(null)
  useEffect(() => { api<BuildPage>('/builds').then(setPage, () => {}) }, [])
  if (!page) return null
  const c = page.challenge
  return (
    <Link href="/build"
      className="group relative mb-8 flex flex-wrap items-center gap-x-5 gap-y-2 overflow-hidden rounded-[16px] bg-gradient-to-r from-pop-1 via-pop-2 to-pop-3 px-5 py-4 text-white shadow-lg sm:px-7 sm:py-5">
      <span className="rounded-full bg-white/20 px-3 py-1 text-xs font-bold uppercase tracking-wide">{t('kicker')}</span>
      <span className="min-w-0 flex-1 text-lg font-extrabold tracking-tight sm:text-xl">
        {(t.locale === 'ru' && c.title_ru) || c.title}
        <span className="ml-2 text-sm font-semibold opacity-80">· {t('heroBuild').toLowerCase()} · {c.entries} {t('entries')}</span>
      </span>
      <span className="flex items-center gap-1.5 rounded-full bg-white px-4 py-2 text-sm font-bold text-terminal transition-transform group-hover:translate-x-0.5">
        {t('join')}<ArrowRight className="size-4" />
      </span>
    </Link>
  )
}
