import { ImageResponse } from 'next/og'
import { PRODUCT } from '@/lib/brand'
import { formatDate, type Locale } from '@/lib/i18n/core'
import { getT } from '@/lib/i18n/server'
import { ogMessages } from '@/lib/i18n/messages/og'

// Share cards (opengraph-image routes). Same palette as the sign-in aside:
// navy ink, paper text, one signal blue. Satori renders these, so every
// element with more than one child needs display:flex and only a CSS subset works.
export const OG_SIZE = { width: 1200, height: 630 }
export const OG_TYPE = 'image/png'

const INK = '#15212b'
const PAPER = '#f4f6f8'
const BLUE = '#6f8ff5'
const MUTED = '#8d9ca8'
const LINE = '#2a3946'
const GREEN = '#4cc38a'

// English only: used for the static `alt` export. The drawn tagline comes from ogMessages.
export const TAGLINE = 'One coding task a day. Bring your own agent.'

export interface OgStat { label: string; value: string | number }

export interface OgCard {
  kicker?: string // small label above the title: the mode or page type
  title: string
  subtitle?: string
  stats?: OgStat[]
  badge?: { text: string; live?: boolean } // top-right pill
}

export function clip(s: string, max: number): string {
  const t = s.trim()
  return t.length > max ? `${t.slice(0, max - 1).trimEnd()}…` : t
}

function Mark({ size }: { size: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 32 32">
      <rect width="32" height="32" rx="8" fill="#243541" />
      <path d="M8.5 11l5 5-5 5" fill="none" stroke={PAPER} strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M16.5 17.5l2.6 2.6 5.4-6.1" fill="none" stroke={BLUE} strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function Frame({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        width: '100%', height: '100%', display: 'flex', flexDirection: 'column', position: 'relative',
        background: INK, color: PAPER, padding: '56px 72px', fontFamily: 'sans-serif',
      }}
    >
      {/* the answered prompt, huge and quiet in the corner */}
      <svg width="520" height="520" viewBox="0 0 32 32" style={{ position: 'absolute', right: -70, bottom: -90, opacity: 0.1 }}>
        <path d="M8.5 11l5 5-5 5" fill="none" stroke={PAPER} strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
        <path d="M16.5 17.5l2.6 2.6 5.4-6.1" fill="none" stroke={BLUE} strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      {children}
    </div>
  )
}

function Header({ badge }: { badge?: OgCard['badge'] }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
      <div style={{ display: 'flex', alignItems: 'center' }}>
        <Mark size={56} />
        <div style={{ display: 'flex', marginLeft: 18, fontSize: 38, fontWeight: 700, letterSpacing: -1.5 }}>{PRODUCT}</div>
      </div>
      {badge && (
        <div
          style={{
            display: 'flex', alignItems: 'center', fontSize: 26, padding: '8px 22px', borderRadius: 999,
            border: `2px solid ${badge.live ? GREEN : LINE}`, color: badge.live ? GREEN : MUTED,
          }}
        >
          {badge.live && <div style={{ display: 'flex', width: 14, height: 14, borderRadius: 7, background: GREEN, marginRight: 12 }} />}
          {badge.text}
        </div>
      )}
    </div>
  )
}

export function cardImage(c: OgCard): ImageResponse {
  const title = clip(c.title, 90)
  const fs = title.length > 60 ? 64 : title.length > 34 ? 78 : 96
  return new ImageResponse(
    (
      <Frame>
        <Header badge={c.badge} />
        <div style={{ display: 'flex', flexDirection: 'column', flex: 1, justifyContent: 'center', padding: '28px 0' }}>
          {c.kicker && (
            <div style={{ display: 'flex', fontSize: 30, color: BLUE, letterSpacing: 4, textTransform: 'uppercase', marginBottom: 20 }}>{c.kicker}</div>
          )}
          <div style={{ display: 'flex', fontSize: fs, fontWeight: 800, lineHeight: 1.08, letterSpacing: -2, maxWidth: 1000 }}>{title}</div>
          {c.subtitle && (
            <div style={{ display: 'flex', fontSize: 34, color: MUTED, marginTop: 24, maxWidth: 980 }}>{clip(c.subtitle, 110)}</div>
          )}
        </div>
        <div style={{ display: 'flex', alignItems: 'flex-end', justifyContent: 'space-between' }}>
          <div style={{ display: 'flex' }}>
            {(c.stats ?? []).slice(0, 4).map((s, i) => (
              <div key={s.label} style={{ display: 'flex', flexDirection: 'column', marginRight: 56, paddingLeft: i === 0 ? 0 : 0 }}>
                <div style={{ display: 'flex', fontSize: 56, fontWeight: 700, letterSpacing: -1.5 }}>{String(s.value)}</div>
                <div style={{ display: 'flex', fontSize: 24, color: MUTED, textTransform: 'uppercase', letterSpacing: 2 }}>{s.label}</div>
              </div>
            ))}
          </div>
          <div style={{ display: 'flex', fontSize: 26, color: MUTED }}>tolerance.cc</div>
        </div>
      </Frame>
    ),
    { ...OG_SIZE },
  )
}

// The fallback for every route: brand and tagline, no data.
export async function defaultImage(): Promise<ImageResponse> {
  const t = await getT(ogMessages)
  return new ImageResponse(
    (
      <Frame>
        <div style={{ display: 'flex', flexDirection: 'column', flex: 1, justifyContent: 'center' }}>
          <div style={{ display: 'flex', alignItems: 'center' }}>
            <Mark size={132} />
            <div style={{ display: 'flex', marginLeft: 36, fontSize: 132, fontWeight: 800, letterSpacing: -6 }}>{PRODUCT}</div>
          </div>
          <div style={{ display: 'flex', fontSize: 52, marginTop: 44, color: PAPER, maxWidth: 900, lineHeight: 1.2 }}>{t('tagline')}</div>
          <div style={{ display: 'flex', fontSize: 30, marginTop: 28, color: MUTED }}>{t('defaultSub')}</div>
        </div>
        <div style={{ display: 'flex', fontSize: 26, color: MUTED }}>tolerance.cc</div>
      </Frame>
    ),
    { ...OG_SIZE },
  )
}

// Wraps a card builder so a missing entity or an API outage yields the default card.
export async function safeCard(build: () => Promise<OgCard | null>): Promise<ImageResponse> {
  try {
    const c = await build()
    return c ? cardImage(c) : await defaultImage()
  } catch {
    return await defaultImage()
  }
}

export function fmtDay(day: string, locale: Locale): string {
  const d = new Date(`${day}T00:00:00Z`)
  if (Number.isNaN(d.getTime())) return day
  return formatDate(locale, d.toISOString(), { day: 'numeric', month: 'long', year: 'numeric' })
}
