import Link from 'next/link'
import { Brand } from '@/components/brand'
import { Button } from '@/components/ui/button'

export default function NotFound() {
  return (
    <main className="mx-auto flex min-h-dvh max-w-xl flex-col px-4 py-6">
      <Brand />
      <div className="flex flex-1 flex-col justify-center py-16">
        <p className="font-mono text-sm text-muted-foreground">404</p>
        <h1 className="display mt-2 text-[2.6rem]">Nothing here</h1>
        <p className="mt-3 text-muted-foreground">This page doesn’t exist. If you followed a link to a proof, it may belong to another account.</p>
        <div className="mt-8 flex flex-wrap gap-2">
          <Button render={<Link href="/app" />} nativeButton={false}>Go to your dashboard</Button>
          <Button variant="outline" render={<Link href="/" />} nativeButton={false}>Home page</Button>
        </div>
      </div>
    </main>
  )
}
