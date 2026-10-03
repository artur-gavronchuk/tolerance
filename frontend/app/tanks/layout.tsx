import type { Metadata } from 'next'
import Link from 'next/link'
import { Brand } from '@/components/brand'
import { SiteHeader } from '@/components/public/site-header'
import { PRODUCT } from '@/lib/brand'
import { getT } from '@/lib/i18n/server'
import { tanksHomeMessages as m } from '@/lib/i18n/messages/tanks-home'

export async function generateMetadata(): Promise<Metadata> {
  const t = await getT(m)
  return {
    title: { default: t('meta.title'), template: `%s · ${t('meta.title')} · ${PRODUCT}` },
    description: t('meta.description'),
  }
}

export default async function TanksLayout({ children }: { children: React.ReactNode }) {
  const t = await getT(m)
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader />
      <main className="flex-1">{children}</main>
      <footer className="border-t border-border">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-3 px-4 py-8 text-sm text-muted-foreground sm:px-6">
          <Brand />
          <span>{t('footer.tagline')}</span>
          <nav className="ml-auto flex gap-5">
            <Link href="/tanks/docs" className="hover:text-foreground">{t('footer.docs')}</Link>
            <Link href="/" className="hover:text-foreground">{t('footer.daily')}</Link>
          </nav>
        </div>
      </footer>
    </div>
  )
}
