'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Code2, Swords, X } from 'lucide-react'
import { useMe } from '@/lib/use-me'
import { useT } from '@/lib/i18n/client'
import { shellMessages } from '@/lib/i18n/messages/shell'

const KEY = 'tolerance.intro.hidden'

const MODES = [
  { icon: Code2, title: 'modeDaily', text: 'modeDailyText', href: '#today' },
  { icon: Swords, title: 'modeTanks', text: 'modeTanksText', href: '/tanks' },
] as const

// Intro for signed-out visitors on the home page: what tolerance is and its modes. Dismissible.
export function Intro() {
  const { me, loading } = useMe()
  const t = useT(shellMessages)
  const [hidden, setHidden] = useState(true)

  useEffect(() => {
    try {
      setHidden(localStorage.getItem(KEY) === '1')
    } catch {
      setHidden(false)
    }
  }, [])

  if (loading || me || hidden) return null

  function hide() {
    setHidden(true)
    try {
      localStorage.setItem(KEY, '1')
    } catch {}
  }

  return (
    <section className="relative mb-8 overflow-hidden rounded-[14px] bg-terminal p-5 text-terminal-foreground sm:p-7">
      <button onClick={hide} aria-label={t('hideIntro')} title={t('hide')}
        className="absolute right-3 top-3 flex size-8 items-center justify-center rounded-full text-terminal-foreground/60 hover:bg-white/10 hover:text-terminal-foreground">
        <X className="size-4" />
      </button>
      <p className="display max-w-2xl pr-8 text-[1.6rem] leading-tight sm:text-[2rem]">
        {t('introTitle')}
      </p>
      <p className="mt-2 max-w-2xl text-[13px] text-terminal-foreground/70 sm:text-base">
        {t('introText')}
      </p>
      <div className="mt-4 grid grid-cols-2 gap-2 sm:mt-5">
        {MODES.map((m) => (
          <Link key={m.href} href={m.href}
            className="group flex flex-col rounded-[10px] border border-white/10 bg-white/[0.04] p-3 sm:p-3.5 transition-colors hover:border-white/25 hover:bg-white/[0.07]">
            <span className="flex items-center gap-2 text-[13px] font-bold sm:text-sm">
              <m.icon className="size-4 text-terminal-accent" />
              {t(m.title)}
            </span>
            <span className="mt-1 hidden text-[13px] leading-snug text-terminal-foreground/65 sm:block">{t(m.text)}</span>
          </Link>
        ))}
      </div>
    </section>
  )
}
