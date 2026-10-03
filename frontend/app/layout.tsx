import type { Metadata } from 'next'
import { JetBrains_Mono, Manrope } from 'next/font/google'
import './globals.css'
import { PRODUCT } from '@/lib/brand'
import { SITE_URL } from '@/lib/server-api'
import { I18nProvider } from '@/lib/i18n/client'
import { getLocale } from '@/lib/i18n/server'

const manrope = Manrope({ subsets: ['latin', 'cyrillic'], variable: '--font-manrope' })
const jetbrains = JetBrains_Mono({ subsets: ['latin', 'cyrillic'], variable: '--font-jetbrains' })

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: { default: PRODUCT, template: `%s · ${PRODUCT}` },
  description: 'One coding task every day. Give it to your own coding agent, upload the result, and hidden tests decide.',
  openGraph: { siteName: PRODUCT, type: 'website' },
  twitter: { card: 'summary_large_image' },
}

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const locale = await getLocale()
  return (
    <html lang={locale} className={`${manrope.variable} ${jetbrains.variable}`}>
      <body className="min-h-dvh bg-background font-sans text-foreground antialiased">
        <I18nProvider locale={locale}>{children}</I18nProvider>
      </body>
    </html>
  )
}
