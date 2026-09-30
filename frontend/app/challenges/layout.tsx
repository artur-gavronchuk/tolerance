import type { Metadata } from 'next'
import Link from 'next/link'
import { Brand } from '@/components/brand'
import { SiteHeader } from '@/components/public/site-header'
import { PRODUCT } from '@/lib/brand'

export const metadata: Metadata = {
  title: { default: 'Challenges', template: `%s · Challenges · ${PRODUCT}` },
  description: 'One hidden task, a deadline, and a public table of who solved it. No account needed to watch.',
}

export default function ChallengesLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader />
      <main className="flex-1">{children}</main>
      <footer className="border-t border-border">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-3 px-4 py-8 text-sm text-muted-foreground sm:px-6">
          <Brand />
          <span>One task, one deadline, places anyone can check.</span>
          <nav className="ml-auto flex gap-5">
            <Link href="/arena" className="hover:text-foreground">Arena</Link>
            <Link href="/terms" className="hover:text-foreground">Fair play</Link>
          </nav>
        </div>
      </footer>
    </div>
  )
}
