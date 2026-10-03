'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { LogOut } from 'lucide-react'
import { Brand } from '@/components/brand'
import { Button } from '@/components/ui/button'
import { post } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { cn } from '@/lib/utils'

const SITE_NAV = [
  { label: 'Today', href: '/' },
  { label: 'Archive', href: '/days' },
  { label: 'Leaderboard', href: '/leaderboard' },
  { label: 'Products', href: '/products' },
  { label: 'Tanks', href: '/tanks' },
]

const TANKS_NAV = [
  { label: 'Live', href: '/tanks' },
  { label: 'Ladder', href: '/tanks/leaderboard' },
  { label: 'Docs', href: '/tanks/docs' },
  { label: 'My bot', href: '/app/tanks' },
]

export function SiteHeader() {
  const { me, loading } = useMe()
  const pathname = usePathname()
  const inTanks = pathname === '/tanks' || pathname.startsWith('/tanks/') || pathname.startsWith('/app/tanks')

  const isActive = (href: string, nav: typeof SITE_NAV) => {
    if (href === '/') return pathname === '/' || pathname.startsWith('/day/')
    if (href === '/tanks' && nav === TANKS_NAV) return pathname === '/tanks'
    return pathname === href || pathname.startsWith(href + '/')
  }

  const link = (item: (typeof SITE_NAV)[number], nav: typeof SITE_NAV) => (
    <Link
      key={item.href}
      href={item.href}
      className={cn(
        'shrink-0 rounded-full px-3 py-1.5 text-sm font-semibold text-muted-foreground transition-colors hover:text-foreground',
        isActive(item.href, nav) && 'bg-muted text-foreground'
      )}
    >
      {item.label}
    </Link>
  )

  async function signOut() {
    await post('/auth/logout')
    window.location.assign('/')
  }

  return (
    <header className="border-b border-border bg-card/80 backdrop-blur-sm">
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-3 px-4 sm:px-6">
        <Brand />
        <nav className="ml-1 hidden items-center gap-1 sm:flex">{SITE_NAV.map((item) => link(item, SITE_NAV))}</nav>
        <div className="ml-auto flex items-center gap-2">
          {loading ? null : me ? (
            <>
              <Link href={`/u/${encodeURIComponent(me.user.handle)}`} title="My profile"
                className="flex h-9 items-center gap-2 rounded-full border border-border bg-card px-3 text-sm hover:border-primary">
                <span className="max-w-[8rem] truncate font-bold">{me.user.handle}</span>
                <span className="font-mono text-xs text-muted-foreground">🔥 {me.streak.current}</span>
              </Link>
              <button onClick={() => void signOut()} aria-label="Sign out" title="Sign out"
                className="flex size-9 items-center justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground">
                <LogOut className="size-4" />
              </button>
            </>
          ) : (
            <Button render={<Link href="/login" />} nativeButton={false}>Sign in</Button>
          )}
        </div>
      </div>
      <div className="flex h-11 items-center gap-1 overflow-x-auto border-t border-border px-4 sm:hidden">
        {SITE_NAV.map((item) => link(item, SITE_NAV))}
      </div>
      {inTanks && (
        <div className="flex h-11 items-center gap-1 overflow-x-auto border-t border-border px-4 sm:px-6">
          <div className="mx-auto flex w-full max-w-6xl items-center gap-1">{TANKS_NAV.map((item) => link(item, TANKS_NAV))}</div>
        </div>
      )}
    </header>
  )
}
