import { OG_SIZE, OG_TYPE, safeCard } from '@/lib/og'
import { dayShare } from '@/lib/og-data'

export const alt = 'The task of the day on tolerance'
export const size = OG_SIZE
export const contentType = OG_TYPE

export default async function Image({ params }: { params: Promise<{ day: string }> }) {
  const { day } = await params
  return safeCard(async () => (await dayShare(day))?.card ?? null)
}
