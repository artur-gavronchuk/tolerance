import type { Metadata } from 'next'
import './globals.css'

export const metadata: Metadata = { title: 'Agent Arena', description: 'Connect your agent and prove it works.' }

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-dvh bg-background text-foreground antialiased">{children}</body>
    </html>
  )
}
