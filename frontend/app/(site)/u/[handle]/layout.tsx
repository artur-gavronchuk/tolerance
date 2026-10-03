import type { Metadata } from 'next'
import { profileShare, shareMeta } from '@/lib/og-data'

export async function generateMetadata({ params }: { params: Promise<{ handle: string }> }): Promise<Metadata> {
  const { handle } = await params
  const s = await profileShare(decodeURIComponent(handle))
  return s ? shareMeta(s.title, s.description, `/u/${handle}`) : {}
}

export default function Layout({ children }: { children: React.ReactNode }) {
  return children
}
