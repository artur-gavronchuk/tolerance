import { OG_SIZE, OG_TYPE, safeCard } from '@/lib/og'
import { profileShare } from '@/lib/og-data'

export const alt = 'A player on tolerance'
export const size = OG_SIZE
export const contentType = OG_TYPE

export default async function Image({ params }: { params: Promise<{ handle: string }> }) {
  const { handle } = await params
  return safeCard(async () => (await profileShare(decodeURIComponent(handle)))?.card ?? null)
}
