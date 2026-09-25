'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { AppShell } from '@/components/app-shell'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useMe } from '@/lib/use-me'
import { friendlyMessage } from '@/lib/api'
import { PRODUCT } from '@/lib/brand'

export default function OwnerLayout({ children }: { children: React.ReactNode }) {
  const { me, loading, error, refresh } = useMe()
  const router = useRouter()
  useEffect(() => {
    if (!loading && (error?.status === 401 || (!me && !error))) router.replace('/login')
  }, [me, loading, error, router])
  if (!loading && error && error.status !== 401) {
    return (
      <div className="mx-auto flex min-h-dvh max-w-md flex-col justify-center px-4">
        <h1 className="display text-3xl">Can’t reach {PRODUCT}</h1>
        <p role="alert" className="mt-3 text-muted-foreground">Loading your account failed: {friendlyMessage(error)}. The API may be restarting.</p>
        <Button className="mt-6 self-start" onClick={() => void refresh()}>Try again</Button>
      </div>
    )
  }
  if (loading || !me) {
    return (
      <div className="min-h-dvh">
        <div className="h-16 border-b border-border bg-card" />
        <div className="mx-auto max-w-6xl space-y-6 px-4 py-10 sm:px-6">
          <Skeleton className="h-4 w-32" />
          <Skeleton className="h-12 w-72 max-w-full" />
          <Skeleton className="h-40 rounded-[16px]" />
        </div>
      </div>
    )
  }
  return <AppShell me={me}>{children}</AppShell>
}
