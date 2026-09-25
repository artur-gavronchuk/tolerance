'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { Brand } from '@/components/brand'
import { Button } from '@/components/ui/button'
import { useMe } from '@/lib/use-me'
import { cn } from '@/lib/utils'

const TANKS_NAV = [
  { label: 'Live', href: '/tanks' },
  { label: 'Leaderboard', href: '/tanks/leaderboard' },
  { label: 'Docs', href: '/tanks/docs' },
]

export function SiteHeader() {
  const { me, loading } = useMe()
  const pathname = usePathname()
  const inTanks = pathname === '/tanks' || pathname.startsWith('/tanks/')

  const tanksLink = (item: (typeof TANKS_NAV)[number], mobile = false) => (
    <Link
      key={item.href}
      href={item.href}
      className={cn(
        'shrink-0 rounded-full px-3 py-1.5 text-sm font-semibold text-muted-foreground transition-colors hover:text-foreground',
        pathname === item.href && 'bg-muted text-foreground',
        mobile && 'px-3 py-1'
      )}
    >
      {item.label}
    </Link>
  )

  return (
    <header className="sticky top-0 z-40 border-b border-border bg-card/85 backdrop-blur-md">
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-3 px-4 sm:px-6">
        <Brand />
        {inTanks ? (
          <nav className="ml-1 hidden items-center gap-1 sm:flex">{TANKS_NAV.map((item) => tanksLink(item))}</nav>
        ) : (
          <Link href="/tanks" className="ml-2 shrink-0 text-sm font-semibold text-muted-foreground transition-colors hover:text-foreground">
            Tanks
          </Link>
        )}
        <div className="ml-auto flex items-center gap-2">
          {loading ? null : me ? (
            <Button render={<Link href="/app" />} nativeButton={false}>Open dashboard</Button>
          ) : (
            <>
              <Button variant="ghost" render={<Link href="/login" />} nativeButton={false}>Sign in</Button>
              <Button render={<Link href="/signup" />} nativeButton={false} className="hidden sm:inline-flex">Create account</Button>
            </>
          )}
        </div>
      </div>
      {inTanks && (
        <div className="flex h-11 items-center gap-1 overflow-x-auto border-t border-border px-4 sm:hidden">
          {TANKS_NAV.map((item) => tanksLink(item, true))}
        </div>
      )}
    </header>
  )
}
