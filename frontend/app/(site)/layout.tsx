import { SiteFooter } from '@/components/public/site-footer'
import { SiteHeader } from '@/components/public/site-header'

export default function SiteLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader />
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6 sm:py-10">{children}</main>
      <SiteFooter />
    </div>
  )
}
