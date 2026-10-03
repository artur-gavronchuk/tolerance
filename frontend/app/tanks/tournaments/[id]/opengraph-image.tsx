import { OG_SIZE, OG_TYPE, safeCard } from '@/lib/og'
import { tournamentShare } from '@/lib/og-data'

export const alt = 'A tanks tournament on tolerance'
export const size = OG_SIZE
export const contentType = OG_TYPE

export default async function Image({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  return safeCard(async () => (await tournamentShare(id))?.card ?? null)
}
