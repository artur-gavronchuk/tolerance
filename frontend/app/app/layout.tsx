'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { AppShell } from '@/components/app-shell'
import { useMe } from '@/lib/use-me'
import { Skeleton } from '@/components/ui/skeleton'

export default function OwnerLayout({ children }: { children: React.ReactNode }) {
  const { me, loading, error } = useMe()
  const router = useRouter()
  useEffect(() => {
    if (!loading && (error?.status === 401 || (!me && !error))) router.replace('/login')
  }, [me, loading, error, router])
  if (loading || !me) return <div className="p-6"><Skeleton className="h-8 w-48" /></div>
  return <AppShell me={me}>{children}</AppShell>
}
