'use client'

import Link from 'next/link'
import { Brand } from '@/components/brand'
import { Button } from '@/components/ui/button'
import { useMe } from '@/lib/use-me'

export function SiteHeader() {
  const { me, loading } = useMe()
  return (
    <header className="sticky top-0 z-40 border-b border-border bg-card/85 backdrop-blur-md">
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-3 px-4 sm:px-6">
        <Brand />
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
    </header>
  )
}
