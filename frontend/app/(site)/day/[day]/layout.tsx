import type { Metadata } from 'next'
import { dayShare, shareMeta } from '@/lib/og-data'

export async function generateMetadata({ params }: { params: Promise<{ day: string }> }): Promise<Metadata> {
  const { day } = await params
  const s = await dayShare(day)
  return s ? shareMeta(s.title, s.description, `/day/${day}`) : {}
}

export default function Layout({ children }: { children: React.ReactNode }) {
  return children
}
