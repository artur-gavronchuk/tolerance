import type { Metadata } from 'next'
import { productShare, shareMeta } from '@/lib/og-data'

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const { slug } = await params
  const s = await productShare(slug)
  return s ? shareMeta(s.title, s.description, `/products/${slug}`) : {}
}

export default function Layout({ children }: { children: React.ReactNode }) {
  return children
}
