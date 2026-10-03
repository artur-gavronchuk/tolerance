// Where to send people after sign-in. Same-site relative paths only, mirroring the backend's safeNext.
const LAST = 'tolerance.lastPath'

export function safeReturnPath(next: string | null | undefined): string | null {
  if (!next || next.length > 512 || !next.startsWith('/') || next.startsWith('//')) return null
  for (let i = 0; i < next.length; i++) {
    const c = next.charCodeAt(i)
    if (c === 92 || c < 0x20 || c === 0x7f) return null
  }
  if (next === '/login' || next.startsWith('/login?') || next.startsWith('/login/')) return null
  return next
}

// The page the visitor was on before they were sent to /login (kept by the site header on every navigation).
export function rememberPath(path: string) {
  try {
    const p = safeReturnPath(path)
    if (p) sessionStorage.setItem(LAST, p)
  } catch {}
}

export function lastPath(): string | null {
  try {
    return safeReturnPath(sessionStorage.getItem(LAST))
  } catch {
    return null
  }
}

export function loginHref(next: string | null | undefined): string {
  const p = safeReturnPath(next)
  return !p || p === '/' ? '/login' : `/login?next=${encodeURIComponent(p)}`
}
