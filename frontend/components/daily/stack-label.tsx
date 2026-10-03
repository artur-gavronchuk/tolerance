import { useLocale } from '@/lib/i18n/client'
import { makeT, type Locale } from '@/lib/i18n/core'
import { dailyMessages } from '@/lib/i18n/messages/daily'

// Display normalizer for the free-text "made with": a TypeScript port of backend/internal/profiles/stack.go
// so every page names a stack the same way ("Claude Code + Sonnet"). Keep the rules in sync with the backend.
const OTHER = 'Other'

const TOOLS: [string, RegExp][] = [
  ['Gemini CLI', /gemini[ -]?cli/],
  ['Claude Code', /claude[ -]?code|claudecode|\bcc\b/],
  ['Codex CLI', /\bcodex\b/],
  ['Cursor', /\bcursor\b/],
  ['Aider', /\baider\b/],
  ['Cline', /\bcline\b|\broo[ -]?code\b/],
  ['Copilot', /copilot/],
  ['Windsurf', /windsurf|\bcascade\b/],
  ['OpenHands', /open[ -]?hands|opendevin/],
  ['Custom', /\bcustom\b|own agent|my agent|my own|\bscripts?\b|home[ -]?(made|grown)|self[ -]?(made|written)|\bby hand\b|\bmanual(ly)?\b|\bnone\b|\bno ai\b|\bno agent\b/],
  ['Claude Code', /\bclaude\b|sub-?agents?/],
]

const MODELS: [string, RegExp][] = [
  ['Opus', /opus/],
  ['Sonnet', /sonnet/],
  ['Haiku', /haiku/],
  ['Fable', /fable/],
  ['GPT-5', /gpt[ -]?5/],
  ['GPT-4', /gpt[ -]?4/],
  ['o-series', /(^|[^a-z0-9])o[134]([ -]?(mini|pro|preview))?($|[^a-z0-9])/],
  ['Gemini', /gemini/],
  ['DeepSeek', /deep[ -]?seek/],
  ['Qwen', /qwen/],
  ['Grok', /grok/],
  ['GPT', /\bgpt\b|chatgpt/],
]

const match = (rules: [string, RegExp][], s: string) => rules.find(([, re]) => re.test(s))?.[0] ?? OTHER

export function stackLabel(madeWith: string): string {
  const s = madeWith.toLowerCase().trim().replace(/[_\t]/g, ' ')
  if (!s) return OTHER
  const tool = match(TOOLS, s)
  const model = match(MODELS, s)
  if (tool === OTHER && model === OTHER) return OTHER
  if (model === OTHER || tool.startsWith(model)) return tool
  if (tool === OTHER) return model
  return `${tool} + ${model}`
}

// What to show for a person's own text: the normalized label, or their raw words when nothing was recognized.
export function displayStack(madeWith: string): string {
  const label = stackLabel(madeWith)
  return label === OTHER && madeWith.trim() ? madeWith.trim() : label
}

// The generic labels in the reader's language; tool and model names stay as they are.
export function localizeStack(label: string, locale: Locale): string {
  if (label === 'Other' || label === 'Custom') return makeT(dailyMessages, locale)(`stack.${label}`)
  return label
}

// The normalized label with the raw text as a tooltip.
export function StackName({ madeWith, empty = '—' }: { madeWith: string; empty?: string }) {
  const locale = useLocale()
  const raw = madeWith.trim()
  if (!raw) return <>{empty}</>
  const label = displayStack(raw)
  return <span title={label === raw ? undefined : raw}>{localizeStack(label, locale)}</span>
}
