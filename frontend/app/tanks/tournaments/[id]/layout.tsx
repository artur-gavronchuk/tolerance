import type { Metadata } from 'next'
import { shareMeta, tournamentShare } from '@/lib/og-data'

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }): Promise<Metadata> {
  const { id } = await params
  const s = await tournamentShare(id)
  return s ? shareMeta(s.title, s.description, `/tanks/tournaments/${id}`) : {}
}

export default function Layout({ children }: { children: React.ReactNode }) {
  return children
}
