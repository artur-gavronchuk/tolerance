'use client'

import { useEffect } from 'react'
import { usePathname, useRouter } from 'next/navigation'
import { loginHref } from '@/components/public/return-path'
import { SiteHeader } from '@/components/public/site-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useMe } from '@/lib/use-me'
import { friendlyMessage } from '@/lib/api'
import { PRODUCT } from '@/lib/brand'

// Signed-in area (the owner's tanks bot). Sends visitors to /login.
export default function OwnerLayout({ children }: { children: React.ReactNode }) {
  const { me, loading, error, refresh } = useMe()
  const router = useRouter()
  const pathname = usePathname()
  useEffect(() => {
    if (!loading && (error?.status === 401 || (!me && !error))) router.replace(loginHref(pathname))
  }, [me, loading, error, router, pathname])
  if (!loading && error && error.status !== 401) {
    return (
      <div className="mx-auto flex min-h-dvh max-w-md flex-col justify-center px-4">
        <h1 className="display text-3xl">Can’t reach {PRODUCT}</h1>
        <p role="alert" className="mt-3 text-muted-foreground">Loading your account failed: {friendlyMessage(error)}. The API may be restarting.</p>
        <Button className="mt-6 self-start" onClick={() => void refresh()}>Try again</Button>
      </div>
    )
  }
  return (
    <div className="min-h-dvh">
      <SiteHeader />
      {loading || !me ? (
        <div className="mx-auto max-w-6xl space-y-6 px-4 py-10 sm:px-6">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-12 w-72 max-w-full" />
          <Skeleton className="h-40 rounded-[16px]" />
        </div>
      ) : (
        <main className="mx-auto max-w-6xl px-4 py-8 sm:px-6 sm:py-10">{children}</main>
      )}
    </div>
  )
}
