import Link from 'next/link'
import { Brand } from '@/components/brand'

export function SiteFooter() {
  return (
    <footer className="border-t border-border">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-3 px-4 py-8 text-sm text-muted-foreground sm:px-6">
        <Brand />
        <span>Daily tasks, weekly products and a tanks arena. Bring your own agent.</span>
        <nav className="ml-auto flex gap-5">
          <Link href="/terms" className="hover:text-foreground">Terms</Link>
        </nav>
      </div>
    </footer>
  )
}
