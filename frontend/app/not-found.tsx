import Link from 'next/link'
import { SiteFooter } from '@/components/public/site-footer'
import { SiteHeader } from '@/components/public/site-header'
import { Button } from '@/components/ui/button'

export default function NotFound() {
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader returnTo="/" />
      <main className="mx-auto flex w-full max-w-6xl flex-1 flex-col justify-center px-4 py-16 sm:px-6">
        <p className="font-mono text-sm text-muted-foreground">404</p>
        <h1 className="display mt-2 text-[2.6rem]">Nothing here</h1>
        <p className="mt-3 max-w-xl text-muted-foreground">This page doesn’t exist, or it has moved. Try one of the three places below.</p>
        <div className="mt-8 flex flex-wrap gap-2">
          <Button render={<Link href="/" />} nativeButton={false}>Today</Button>
          <Button variant="outline" render={<Link href="/products" />} nativeButton={false}>Products</Button>
          <Button variant="outline" render={<Link href="/tanks" />} nativeButton={false}>Tanks</Button>
        </div>
      </main>
      <SiteFooter />
    </div>
  )
}
