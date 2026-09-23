'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { useMe } from '@/lib/use-me'

export default function Home() {
  const { me, loading } = useMe()
  const router = useRouter()
  useEffect(() => {
    if (!loading) router.replace(me ? '/app' : '/login')
  }, [me, loading, router])
  return <p className="p-6 text-sm text-muted-foreground">Loading…</p>
}
