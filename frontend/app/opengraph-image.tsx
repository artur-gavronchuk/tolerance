import { defaultImage, OG_SIZE, OG_TYPE, TAGLINE } from '@/lib/og'

export const alt = `tolerance. ${TAGLINE}`
export const size = OG_SIZE
export const contentType = OG_TYPE

export default async function Image() {
  return defaultImage()
}
