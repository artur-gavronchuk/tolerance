'use client'

import { usePathname } from 'next/navigation'

// Where to send people after sign-in. Same-site relative paths only, mirroring the backend's safeNext.
export function safeReturnPath(next: string | null | undefined): string | null {
  if (!next || next.length > 512 || !next.startsWith('/') || next.startsWith('//')) return null
  for (let i = 0; i < next.length; i++) {
    const c = next.charCodeAt(i)
    if (c === 92 || c < 0x20 || c === 0x7f) return null
  }
  if (next === '/login' || next.startsWith('/login?') || next.startsWith('/login/')) return null
  return next
}

export function loginHref(next: string | null | undefined): string {
  const p = safeReturnPath(next)
  return !p || p === '/' ? '/login' : `/login?next=${encodeURIComponent(p)}`
}

// A /login link that returns to the page it is rendered on.
export function useLoginHref(): string {
  return loginHref(usePathname())
}
