import type { Metadata } from 'next'
import { botShare, shareMeta } from '@/lib/og-data'

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }): Promise<Metadata> {
  const { id } = await params
  const s = await botShare(id)
  return s ? shareMeta(s.title, s.description, `/tanks/bots/${id}`) : {}
}

export default function Layout({ children }: { children: React.ReactNode }) {
  return children
}
