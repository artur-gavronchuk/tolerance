'use client'

import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { LogOut } from 'lucide-react'
import { BrandMark } from '@/components/brand'
import { PRESENCE_LABEL, StatusDot } from '@/components/status-dot'
import { post } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { Me } from '@/lib/types'
import { PRODUCT } from '@/lib/brand'

const NAV = [
  { label: 'Overview', href: '/app' },
  { label: 'Proof task', href: '/app/proofs/new' },
  { label: 'Tanks', href: '/app/tanks' },
  { label: 'Connector', href: '/app/agent/connect' },
]

export function AppShell({ me, children }: { me: Me; children: React.ReactNode }) {
  const pathname = usePathname()
  const router = useRouter()
  async function signOut() {
    await post('/auth/logout')
    router.replace('/login')
  }
  const active = (href: string) => (href === '/app' ? pathname === '/app' : pathname.startsWith(href))
  const a = me.agent
  const nav = (
    <nav className="flex h-full items-stretch">
      {NAV.map((item) => (
        <Link key={item.href} href={item.href}
          className={cn('relative flex items-center px-3 text-sm font-semibold text-muted-foreground transition-colors hover:text-foreground',
            active(item.href) && 'text-foreground after:absolute after:inset-x-3 after:bottom-0 after:h-0.5 after:rounded-full after:bg-primary')}>
          {item.label}
        </Link>
      ))}
    </nav>
  )
  return (
    <div className="min-h-dvh">
      <header className="sticky top-0 z-40 border-b border-border bg-card/90 backdrop-blur-md">
        <div className="mx-auto flex h-16 max-w-6xl items-stretch gap-4 px-4 sm:px-6">
          <Link href="/app" aria-label={`${PRODUCT} home`} className="flex items-center gap-2.5">
            <BrandMark />
            <span className="hidden text-[1.05rem] font-extrabold tracking-[-0.04em] lg:inline">{PRODUCT}</span>
          </Link>
          <div className="hidden md:flex md:pl-4">{nav}</div>
          <div className="ml-auto flex items-center gap-2">
            {a && (
              <Link href="/app/agent/connect" className="flex h-9 items-center gap-2 rounded-full border border-border bg-card px-3 text-sm hover:bg-muted"
                title={PRESENCE_LABEL[a.stage]}>
                <StatusDot stage={a.stage} />
                <span className="max-w-[9rem] truncate font-bold">{a.name}</span>
              </Link>
            )}
            <span className="hidden text-xs text-muted-foreground xl:inline">{me.user.email}</span>
            <button onClick={signOut} aria-label="Sign out" title="Sign out"
              className="flex size-9 items-center justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground">
              <LogOut className="size-4" />
            </button>
          </div>
        </div>
        <div className="flex h-11 border-t border-border px-1 md:hidden">{nav}</div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-8 sm:px-6 sm:py-10">{children}</main>
    </div>
  )
}
