// First-party analytics beacon: batched POST /api/v1/events, no cookies, no third parties.
// Signed-out visitors get a random id in localStorage; with Do Not Track / Global Privacy Control on, none
// is created (and the server drops signed-out events anyway).

export interface AnalyticsEvent {
  name: string // 'page_view', 'daily.download' or 'ui.<action>'
  path?: string // route pattern, never the real URL
  ref?: string // referrer host
  utm?: string
  entry?: boolean
}

const KEY = 'tl_aid'
const FLUSH_MS = 3000
let queue: AnalyticsEvent[] = []
let timer: ReturnType<typeof setTimeout> | null = null

function optedOut() {
  const nav = navigator as Navigator & { globalPrivacyControl?: boolean }
  return nav.doNotTrack === '1' || nav.globalPrivacyControl === true
}

function anonId() {
  if (optedOut()) return ''
  try {
    let id = localStorage.getItem(KEY)
    if (!id || !/^[a-f0-9]{32}$/.test(id)) {
      id = Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, '0')).join('')
      localStorage.setItem(KEY, id)
    }
    return id
  } catch {
    return ''
  }
}

export function flush() {
  if (timer) clearTimeout(timer)
  timer = null
  if (!queue.length) return
  const body = JSON.stringify({ anon_id: anonId(), events: queue.slice(0, 20) })
  queue = queue.slice(20)
  try {
    const ok = navigator.sendBeacon?.('/api/v1/events', new Blob([body], { type: 'application/json' }))
    if (!ok) void fetch('/api/v1/events', { method: 'POST', body, keepalive: true, headers: { 'Content-Type': 'application/json' } }).catch(() => {})
  } catch {}
  if (queue.length) flush()
}

export function track(e: AnalyticsEvent) {
  if (typeof window === 'undefined') return
  queue.push(e)
  if (queue.length >= 10) flush()
  else if (!timer) timer = setTimeout(flush, FLUSH_MS)
}

// Route pattern from the pathname and the route params: /day/2026-10-01 -> /day/[day].
export function routePattern(pathname: string, params: Record<string, string | string[] | undefined>) {
  let p = pathname
  for (const [k, v] of Object.entries(params)) {
    if (typeof v === 'string' && v) p = p.split('/').map((s) => (s === v || s === encodeURIComponent(v) ? `[${k}]` : s)).join('/')
    else if (Array.isArray(v) && v.length) p = p.replace('/' + v.join('/'), `/[...${k}]`)
  }
  return p
}

// The first page view of a browser session carries where the visitor came from (host only, never a URL).
export function entrySource() {
  try {
    if (sessionStorage.getItem('tl_entry')) return null
    sessionStorage.setItem('tl_entry', '1')
  } catch {
    return null
  }
  let ref = ''
  try {
    const h = document.referrer ? new URL(document.referrer).hostname : ''
    ref = h && h !== location.hostname ? h : ''
  } catch {}
  const utm = new URLSearchParams(location.search).get('utm_source')?.slice(0, 40) ?? ''
  return { ref, utm }
}
