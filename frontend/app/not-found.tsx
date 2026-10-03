import Link from 'next/link'
import { SiteFooter } from '@/components/public/site-footer'
import { SiteHeader } from '@/components/public/site-header'
import { Button } from '@/components/ui/button'
import { getT } from '@/lib/i18n/server'
import { shellMessages } from '@/lib/i18n/messages/shell'

export default async function NotFound() {
  const t = await getT(shellMessages)
  return (
    <div className="flex min-h-dvh flex-col">
      <SiteHeader returnTo="/" />
      <main className="mx-auto flex w-full max-w-6xl flex-1 flex-col justify-center px-4 py-16 sm:px-6">
        <p className="font-mono text-sm text-muted-foreground">404</p>
        <h1 className="display mt-2 text-[2.6rem]">{t('notFoundTitle')}</h1>
        <p className="mt-3 max-w-xl text-muted-foreground">{t('notFoundText')}</p>
        <div className="mt-8 flex flex-wrap gap-2">
          <Button render={<Link href="/" />} nativeButton={false}>{t('today')}</Button>
          <Button variant="outline" render={<Link href="/products" />} nativeButton={false}>{t('products')}</Button>
          <Button variant="outline" render={<Link href="/tanks" />} nativeButton={false}>{t('tanks')}</Button>
        </div>
      </main>
      <SiteFooter />
    </div>
  )
}
