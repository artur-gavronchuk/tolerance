import type { Metadata } from 'next'
import { matchShare, shareMeta } from '@/lib/og-data'

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }): Promise<Metadata> {
  const { id } = await params
  const s = await matchShare(id)
  return s ? shareMeta(s.title, s.description, `/tanks/matches/${id}`) : {}
}

export default function Layout({ children }: { children: React.ReactNode }) {
  return children
}
