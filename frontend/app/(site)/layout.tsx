import Link from 'next/link'
import { Brand } from '@/components/brand'
import { SiteHeader } from '@/components/public/site-header'

export default function SiteLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader />
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6 sm:py-10">{children}</main>
      <footer className="border-t border-border">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-3 px-4 py-8 text-sm text-muted-foreground sm:px-6">
          <Brand />
          <span>One coding task every day. Bring your own agent.</span>
          <nav className="ml-auto flex gap-5">
            <Link href="/terms" className="hover:text-foreground">Terms</Link>
          </nav>
        </div>
      </footer>
    </div>
  )
}
