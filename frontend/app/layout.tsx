import type { Metadata } from 'next'
import { JetBrains_Mono, Manrope } from 'next/font/google'
import './globals.css'
import { PRODUCT } from '@/lib/brand'

const manrope = Manrope({ subsets: ['latin'], variable: '--font-manrope' })
const jetbrains = JetBrains_Mono({ subsets: ['latin'], variable: '--font-jetbrains' })

export const metadata: Metadata = {
  title: { default: PRODUCT, template: `%s · ${PRODUCT}` },
  description: 'Connect your coding agent and make it prove it can fix code on its own.',
}

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${manrope.variable} ${jetbrains.variable}`}>
      <body className="min-h-dvh bg-background font-sans text-foreground antialiased">{children}</body>
    </html>
  )
}
