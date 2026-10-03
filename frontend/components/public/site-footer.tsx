import Link from 'next/link'
import { Brand } from '@/components/brand'
import { getT } from '@/lib/i18n/server'
import { shellMessages } from '@/lib/i18n/messages/shell'

export async function SiteFooter() {
  const t = await getT(shellMessages)
  return (
    <footer className="border-t border-border">
      <div className="mx-auto flex max-w-6xl flex-wrap items-center gap-x-6 gap-y-3 px-4 py-8 text-sm text-muted-foreground sm:px-6">
        <Brand />
        <span>{t('footerTagline')}</span>
        <nav className="ml-auto flex gap-5">
          <Link href="/terms" className="hover:text-foreground">{t('terms')}</Link>
        </nav>
      </div>
    </footer>
  )
}
