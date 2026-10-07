import { Gamepad2, Globe, Smartphone, Wrench, type LucideIcon } from 'lucide-react'
import type { Locale } from './i18n/core'
import type { BuildFormat, BuildText } from './types'

// The look of each format: one flat poster colour (text on it is always text-terminal, navy in both themes).
export const FORMATS: Record<BuildFormat, { icon: LucideIcon; bg: string }> = {
  game: { icon: Gamepad2, bg: 'bg-pop-2' },
  site: { icon: Globe, bg: 'bg-pop-4' },
  mobile: { icon: Smartphone, bg: 'bg-pop-5' },
  tool: { icon: Wrench, bg: 'bg-pop-6' },
}

// tx picks the reader's language, falling back to English.
export const tx = (t: BuildText | undefined, locale: Locale) => (t ? (locale === 'ru' && t.ru) || t.en : '')

// daysLeft is whole days until iso, rounded up (0 once it has passed).
export const daysLeft = (iso: string, now = Date.now()) => Math.max(0, Math.ceil((Date.parse(iso) - now) / 86_400_000))
