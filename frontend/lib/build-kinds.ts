import { Gamepad2, Globe, Smartphone, Wrench, type LucideIcon } from 'lucide-react'
import type { Locale } from './i18n/core'
import type { BuildFormat, BuildText } from './types'

// The look of each format: the same gradient on its card, its hero and its badges.
export const FORMATS: Record<BuildFormat, { icon: LucideIcon; gradient: string; text: string; glow: string }> = {
  game: { icon: Gamepad2, gradient: 'from-pop-1 via-pop-2 to-pop-3', text: 'text-pop-2', glow: 'bg-pop-2/30' },
  site: { icon: Globe, gradient: 'from-pop-4 via-pop-1 to-pop-2', text: 'text-pop-4', glow: 'bg-pop-4/30' },
  mobile: { icon: Smartphone, gradient: 'from-pop-5 via-pop-4 to-pop-1', text: 'text-pop-5', glow: 'bg-pop-5/30' },
  tool: { icon: Wrench, gradient: 'from-pop-6 via-pop-3 to-pop-2', text: 'text-pop-3', glow: 'bg-pop-3/30' },
}

// tx picks the reader's language, falling back to English.
export const tx = (t: BuildText | undefined, locale: Locale) => (t ? (locale === 'ru' && t.ru) || t.en : '')

// daysLeft is whole days until iso, rounded up (0 once it has passed).
export const daysLeft = (iso: string, now = Date.now()) => Math.max(0, Math.ceil((Date.parse(iso) - now) / 86_400_000))
