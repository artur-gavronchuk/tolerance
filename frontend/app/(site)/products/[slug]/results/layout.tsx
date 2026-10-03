import type { Metadata } from 'next'
import { getT } from '@/lib/i18n/server'
import { productsMessages } from '@/lib/i18n/messages/products'
import { productShare, shareMeta } from '@/lib/og-data'

export async function generateMetadata({ params }: { params: Promise<{ slug: string }> }): Promise<Metadata> {
  const { slug } = await params
  const [s, t] = await Promise.all([productShare(slug), getT(productsMessages)])
  return s ? shareMeta(t('res.titlePrefix', { title: s.title }), s.description, `/products/${slug}/results`) : {}
}

export default function Layout({ children }: { children: React.ReactNode }) {
  return children
}
