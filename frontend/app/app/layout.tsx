'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { AppShell } from '@/components/app-shell'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useMe } from '@/lib/use-me'

export default function OwnerLayout({ children }: { children: React.ReactNode }) {
  const { me, loading, error, refresh } = useMe()
  const router = useRouter()
  useEffect(() => {
    if (!loading && (error?.status === 401 || (!me && !error))) router.replace('/login')
  }, [me, loading, error, router])
  if (!loading && error && error.status !== 401) {
    return (
      <div className="p-6">
        <p role="alert" className="text-sm text-destructive">Could not load your account: {error.message}</p>
        <p className="mt-1 text-sm text-muted-foreground">The API may be down or restarting.</p>
        <Button variant="outline" size="sm" className="mt-3" onClick={() => void refresh()}>Try again</Button>
      </div>
    )
  }
  if (loading || !me) return <div className="p-6"><Skeleton className="h-8 w-48" /></div>
  return <AppShell me={me}>{children}</AppShell>
}
