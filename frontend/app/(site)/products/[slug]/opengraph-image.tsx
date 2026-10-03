import { OG_SIZE, OG_TYPE, safeCard } from '@/lib/og'
import { productShare } from '@/lib/og-data'

export const alt = 'Product of the week on tolerance'
export const size = OG_SIZE
export const contentType = OG_TYPE

export default async function Image({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params
  return safeCard(async () => (await productShare(slug))?.card ?? null)
}
