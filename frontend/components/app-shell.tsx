'use client'

import Link from 'next/link'
import { usePathname, useRouter } from 'next/navigation'
import { Button } from '@/components/ui/button'
import { post } from '@/lib/api'
import { cn } from '@/lib/utils'
import type { Me } from '@/lib/types'

const NAV = [
  { label: 'Home', href: '/app' },
  { label: 'Connect', href: '/app/agent/connect' },
]

export function AppShell({ me, children }: { me: Me; children: React.ReactNode }) {
  const pathname = usePathname()
  const router = useRouter()
  async function signOut() {
    await post('/auth/logout')
    router.replace('/login')
  }
  return (
    <div className="min-h-dvh bg-background">
      <header className="sticky top-0 z-40 border-b border-border bg-background/95 backdrop-blur-sm">
        <div className="mx-auto flex h-14 max-w-5xl items-center gap-4 px-4">
          <Link href="/app" className="text-sm font-semibold">{me.agent?.name ?? 'Agent Arena'}</Link>
          <nav className="flex items-center gap-1">
            {NAV.map((item) => (
              <Link key={item.href} href={item.href}
                className={cn('rounded-md px-3 py-1.5 text-sm text-muted-foreground hover:bg-muted hover:text-foreground',
                  (item.href === '/app' ? pathname === '/app' : pathname.startsWith(item.href)) && 'bg-muted text-foreground')}>
                {item.label}
              </Link>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-2">
            <span className="hidden text-xs text-muted-foreground sm:inline">{me.user.email}</span>
            <Button variant="outline" size="sm" onClick={signOut}>Sign out</Button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-6">{children}</main>
    </div>
  )
}
