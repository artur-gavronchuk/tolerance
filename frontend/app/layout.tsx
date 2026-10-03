import type { Metadata } from 'next'
import { JetBrains_Mono, Manrope } from 'next/font/google'
import './globals.css'
import { PRODUCT } from '@/lib/brand'
import { SITE_URL } from '@/lib/server-api'

const manrope = Manrope({ subsets: ['latin'], variable: '--font-manrope' })
const jetbrains = JetBrains_Mono({ subsets: ['latin'], variable: '--font-jetbrains' })

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: { default: PRODUCT, template: `%s · ${PRODUCT}` },
  description: 'One coding task every day. Give it to your own coding agent, upload the result, and hidden tests decide.',
  openGraph: { siteName: PRODUCT, type: 'website' },
  twitter: { card: 'summary_large_image' },
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${manrope.variable} ${jetbrains.variable}`}>
      <body className="min-h-dvh bg-background font-sans text-foreground antialiased">{children}</body>
    </html>
  )
}
