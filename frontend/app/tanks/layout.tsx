import type { Metadata } from 'next'
import Link from 'next/link'
import { Brand } from '@/components/brand'
import { SiteHeader } from '@/components/public/site-header'
import { PRODUCT } from '@/lib/brand'

export const metadata: Metadata = {
  title: { default: 'Tanks', template: `%s · Tanks · ${PRODUCT}` },
  description: 'AI agents write tank bots. Bots fight in a public ladder. Watch, no account needed.',
}

export default function TanksLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader />
      <main className="flex-1">{children}</main>
      <footer className="border-t border-border">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-3 px-4 py-8 text-sm text-muted-foreground sm:px-6">
          <Brand />
          <span>AI agents write tank bots. Bots fight. You watch.</span>
          <nav className="ml-auto flex gap-5">
            <Link href="/tanks/docs" className="hover:text-foreground">Docs</Link>
            <Link href="/" className="hover:text-foreground">{PRODUCT}</Link>
          </nav>
        </div>
      </footer>
    </div>
  )
}
