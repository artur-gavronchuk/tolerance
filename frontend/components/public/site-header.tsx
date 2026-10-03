'use client'

import { useEffect, useRef } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { LogOut } from 'lucide-react'
import { Brand } from '@/components/brand'
import { loginHref } from '@/components/public/return-path'
import { Button } from '@/components/ui/button'
import { post } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import { cn } from '@/lib/utils'
import { useT } from '@/lib/i18n/client'
import { navMessages } from '@/lib/i18n/messages/nav'
import { LocaleSwitch } from '@/components/public/locale-switch'
import { NotificationBell } from '@/components/public/notification-bell'

type NavKey = keyof typeof navMessages.en

// The three modes first, then the cross-cutting pages.
const SITE_NAV = [
  { label: 'today' as NavKey, href: '/' },
  { label: 'products' as NavKey, href: '/products' },
  { label: 'tanks' as NavKey, href: '/tanks' },
  { label: 'leaderboard' as NavKey, href: '/leaderboard' },
  { label: 'agents' as NavKey, href: '/agents' },
  { label: 'archive' as NavKey, href: '/days' },
]

const TANKS_NAV = [
  { label: 'live' as NavKey, href: '/tanks' },
  { label: 'ladder' as NavKey, href: '/tanks/leaderboard' },
  { label: 'tournaments' as NavKey, href: '/tanks/tournaments' },
  { label: 'docs' as NavKey, href: '/tanks/docs' },
  { label: 'myBot' as NavKey, href: '/app/tanks' },
]

// returnTo overrides the post-sign-in destination, for pages whose own URL is not a place to return to (404).
export function SiteHeader({ returnTo }: { returnTo?: string }) {
  const { me, loading } = useMe()
  const t = useT(navMessages)
  const pathname = usePathname()
  const stripRef = useRef<HTMLDivElement>(null)
  const inTanks = pathname === '/tanks' || pathname.startsWith('/tanks/') || pathname.startsWith('/app/tanks')

  // Keep the active tab of the mobile strip in view.
  useEffect(() => {
    const strip = stripRef.current
    const active = strip?.querySelector<HTMLElement>('[data-active="true"]')
    if (!strip || !active) return
    strip.scrollLeft = Math.max(0, active.offsetLeft - (strip.clientWidth - active.offsetWidth) / 2)
  }, [pathname, me?.can_admin])

  const isActive = (href: string, nav: typeof SITE_NAV) => {
    if (href === '/') return pathname === '/' || pathname.startsWith('/day/')
    if (href === '/tanks' && nav === TANKS_NAV) return pathname === '/tanks'
    if (href === '/tanks') return inTanks
    return pathname === href || pathname.startsWith(href + '/')
  }

  const link = (item: (typeof SITE_NAV)[number], nav: typeof SITE_NAV) => (
    <Link
      key={item.href}
      href={item.href}
      data-active={isActive(item.href, nav)}
      className={cn(
        'shrink-0 rounded-full px-3 py-1.5 text-sm font-semibold text-muted-foreground transition-colors hover:text-foreground',
        isActive(item.href, nav) && 'bg-muted text-foreground'
      )}
    >
      {t(item.label)}
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
          <LocaleSwitch />
          {loading ? null : me ? (
            <>
              {me.can_admin && (
                <Link href="/admin" className="hidden h-9 items-center rounded-full px-3 text-sm font-semibold text-muted-foreground hover:bg-muted hover:text-foreground sm:flex">{t('admin')}</Link>
              )}
              <NotificationBell />
              <Link href={`/u/${encodeURIComponent(me.user.handle)}`} title={t('myProfile')}
                className="flex h-9 items-center gap-2 rounded-full border border-border bg-card px-3 text-sm hover:border-primary">
                <span className="max-w-[8rem] truncate font-bold">{me.user.handle}</span>
                <span className="font-mono text-xs text-muted-foreground" title={t('streakHelp')}
                  aria-label={t('streakLabel', { n: me.streak.current, help: t('streakHelp') })}>🔥 {me.streak.current}</span>
              </Link>
              <button onClick={() => void signOut()} aria-label={t('signOut')} title={t('signOut')}
                className="flex size-9 items-center justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground">
                <LogOut className="size-4" />
              </button>
            </>
          ) : (
            <Button render={<Link href={loginHref(returnTo ?? pathname)} />} nativeButton={false}>{t('signIn')}</Button>
          )}
        </div>
      </div>
      <div className="border-t border-border sm:hidden">
        <div ref={stripRef} className="flex h-11 items-center gap-1 overflow-x-auto pl-4 [mask-image:linear-gradient(to_right,#000_calc(100%-2rem),transparent)] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {SITE_NAV.map((item) => link(item, SITE_NAV))}
          {me?.can_admin && link({ label: 'admin', href: '/admin' }, SITE_NAV)}
          {/* As wide as the fade, so the last tab clears it at the end of the scroll. */}
          <span aria-hidden className="w-8 shrink-0" />
        </div>
      </div>
      {inTanks && (
        <div className="border-t border-border">
          <div className="mx-auto flex h-11 max-w-6xl items-center overflow-x-auto px-4 sm:px-6">
            <div className="-ml-3 flex items-center gap-1">{TANKS_NAV.map((item) => link(item, TANKS_NAV))}</div>
          </div>
        </div>
      )}
    </header>
  )
}
