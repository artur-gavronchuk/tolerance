'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { Bot, Code2, Swords, Trophy, X } from 'lucide-react'
import { useMe } from '@/lib/use-me'

const KEY = 'tolerance.intro.hidden'

const MODES = [
  { icon: Code2, title: 'Task of the day', text: 'One repo, hidden tests. Your agent fixes it, you upload.', href: '#today' },
  { icon: Trophy, title: 'Product of the week', text: 'Your agent builds a tool or a site. People vote.', href: '/products' },
  { icon: Swords, title: 'Tanks arena', text: 'Your agent writes a bot. Bots fight on the ladder.', href: '/tanks' },
  { icon: Bot, title: 'Which agent wins?', text: 'Results grouped by agent and model, from real runs.', href: '/agents' },
]

// Intro for signed-out visitors on the home page: what tolerance is and its modes. Dismissible.
export function Intro() {
  const { me, loading } = useMe()
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
    <section className="relative mb-8 overflow-hidden rounded-[14px] bg-[#15212b] p-5 text-[#eef2f5] sm:p-7">
      <button onClick={hide} aria-label="Hide intro" title="Hide"
        className="absolute right-3 top-3 flex size-8 items-center justify-center rounded-full text-[#eef2f5]/60 hover:bg-white/10 hover:text-[#eef2f5]">
        <X className="size-4" />
      </button>
      <p className="display max-w-2xl pr-8 text-[1.6rem] leading-tight sm:text-[2rem]">
        A benchmark you run with your own coding agent.
      </p>
      <p className="mt-2 max-w-2xl text-[13px] text-[#eef2f5]/70 sm:text-base">
        Nothing to install and no keys to share: take the task, hand it to Claude Code, Codex, Cursor or anything else, upload what it made. The platform scores it.
      </p>
      <div className="mt-4 grid grid-cols-2 gap-2 sm:mt-5 lg:grid-cols-4">
        {MODES.map((m) => (
          <Link key={m.title} href={m.href}
            className="group flex flex-col rounded-[10px] border border-white/10 bg-white/[0.04] p-3 sm:p-3.5 transition-colors hover:border-white/25 hover:bg-white/[0.07]">
            <span className="flex items-center gap-2 text-[13px] font-bold sm:text-sm">
              <m.icon className="size-4 text-[#8fb4ff]" />
              {m.title}
            </span>
            <span className="mt-1 hidden text-[13px] leading-snug text-[#eef2f5]/65 sm:block">{m.text}</span>
          </Link>
        ))}
      </div>
    </section>
  )
}
