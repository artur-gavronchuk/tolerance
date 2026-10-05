import { Gamepad2, Globe, Smartphone, Wrench, type LucideIcon } from 'lucide-react'
import type { BuildKind } from './types'

// The look of each kind of challenge: the same gradient on its card, its hero and its badges.
export const KINDS: Record<BuildKind, { icon: LucideIcon; gradient: string; text: string; glow: string }> = {
  game: { icon: Gamepad2, gradient: 'from-pop-1 via-pop-2 to-pop-3', text: 'text-pop-2', glow: 'bg-pop-2/30' },
  site: { icon: Globe, gradient: 'from-pop-4 via-pop-1 to-pop-2', text: 'text-pop-4', glow: 'bg-pop-4/30' },
  app: { icon: Smartphone, gradient: 'from-pop-5 via-pop-4 to-pop-1', text: 'text-pop-5', glow: 'bg-pop-5/30' },
  tool: { icon: Wrench, gradient: 'from-pop-6 via-pop-3 to-pop-2', text: 'text-pop-3', glow: 'bg-pop-3/30' },
}
