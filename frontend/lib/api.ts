import type {
  AdminEvent, AdminFunnel, AdminPulse, ModItem, ModLogItem, ModUser, NotificationList, Recap, CompareJudged, CompareNext, ProductDetail, ProductEntry, ProductList, ProductResults, ProductSourceFile, ProductTask, SeasonDetail, SeasonView, Showcase, StackRow, TournamentView,
} from './types'

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string, public retryAfterSec?: number, public fields?: { path: string; code: string }[]) {
    super(message)
  }
}

// Parses a Retry-After header (seconds, per RFC 9110; the backend never sends
// the HTTP-date form) into a positive integer, or undefined if absent/invalid.
function retryAfterSeconds(res: Response): number | undefined {
  const raw = res.headers.get('Retry-After')
  if (!raw) return undefined
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? Math.round(n) : undefined
}

const BASE = '/api/v1'

async function throwProblem(res: Response): Promise<never> {
  let body: { code?: string; message?: string; fields?: { path: string; code: string }[] } | null = null
  try {
    body = await res.json()
  } catch {}
  throw new ApiError(res.status, body?.code ?? 'error', body?.message ?? res.statusText, retryAfterSeconds(res), body?.fields)
}

// Like fetch, but same-origin under /api/v1 with the session cookie attached
// and no JSON parsing. Callers that need the raw Response — a binary or
// streamed body, like a gzip replay — use this instead of api().
export async function apiRaw(path: string, init: RequestInit = {}): Promise<Response> {
  const res = await fetch(`${BASE}${path}`, { ...init, credentials: 'include' })
  if (!res.ok) await throwProblem(res)
  return res
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) },
  })
  if (!res.ok) await throwProblem(res)
  if (res.status === 204) return undefined as T
  const text = await res.text()
  return (text ? JSON.parse(text) : null) as T
}

export const post = <T,>(path: string, body?: unknown) =>
  api<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })

// Uploads go as multipart/form-data, so no JSON Content-Type header.
export async function upload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(`${BASE}${path}`, { method: 'POST', body: form, credentials: 'include' })
  if (!res.ok) await throwProblem(res)
  return (await res.json()) as T
}

// Product tasks.
export const products = {
  list: () => api<ProductList>('/products'),
  get: (slug: string) => api<ProductDetail>(`/products/${encodeURIComponent(slug)}`),
  results: (slug: string, limit?: number) => api<ProductResults>(`/products/${encodeURIComponent(slug)}/results${limit ? `?limit=${limit}` : ''}`),
  submit: (slug: string, form: FormData) => upload<ProductEntry>(`/products/${encodeURIComponent(slug)}/entries`, form),
  vote: (entryId: string) => post<ProductEntry>(`/product-entries/${encodeURIComponent(entryId)}/vote`),
  unvote: (entryId: string) => api<ProductEntry>(`/product-entries/${encodeURIComponent(entryId)}/vote`, { method: 'DELETE' }),
  compareNext: (slug: string) => api<CompareNext>(`/products/${encodeURIComponent(slug)}/compare/next`),
  compare: (slug: string, a: string, b: string, winner: 'a' | 'b' | 'tie') =>
    post<CompareJudged>(`/products/${encodeURIComponent(slug)}/compare`, { a, b, winner }),
  source: (entryId: string) => api<{ files: ProductSourceFile[] }>(`/product-entries/${encodeURIComponent(entryId)}/source`).then((r) => r.files),
  // Admins and local dev runs: end uploads now (final: end voting too), or open the task again for some days.
  close: (slug: string, final = false) => post<ProductDetail>(`/products/${encodeURIComponent(slug)}/close`, { final }),
  startNext: () => post<ProductDetail>('/products/start-next'),
  reopen: (slug: string, days = 7) => post<ProductDetail>(`/products/${encodeURIComponent(slug)}/reopen`, { days }),
}

// Agent stacks: which tool + model combinations do best at the daily task.
export const stacks = {
  overall: (days?: number) => api<{ items: StackRow[] }>(`/stacks${days ? `?days=${days}` : ''}`).then((r) => r.items),
  forDay: (day: string) => api<{ items: StackRow[] }>(`/daily/${day}/stacks`).then((r) => r.items),
}

// Tanks showcase, seasons and tournaments (public reads; starting a tournament needs an admin or the dev login).
export const tanks = {
  showcase: () => api<Showcase>('/tanks/showcase'),
  seasons: () => api<{ items: SeasonView[] }>('/tanks/seasons').then((r) => r.items),
  season: (id: string) => api<SeasonDetail>(`/tanks/seasons/${encodeURIComponent(id)}`),
  tournaments: (status = '') =>
    api<{ items: TournamentView[]; now: string }>(`/tanks/tournaments${status ? `?status=${status}` : ''}`),
  tournament: (id: string) => api<TournamentView>(`/tanks/tournaments/${encodeURIComponent(id)}`),
  startTournament: (size?: number) => post<TournamentView>('/tanks/tournaments', size ? { size } : {}),
}

export const admin = {
  pulse: () => api<AdminPulse>('/admin/pulse'),
  funnel: () => api<AdminFunnel>('/admin/funnel'),
  recent: () => api<{ items: AdminEvent[] }>('/admin/recent').then((r) => r.items),
}

export type ModKind = 'entry' | 'submission' | 'bot'
export const moderation = {
  users: (q: string) => api<{ items: ModUser[] }>(`/admin/moderation/users?q=${encodeURIComponent(q)}`).then((r) => r.items),
  items: (userId: string) => api<{ items: ModItem[] }>(`/admin/moderation/users/${encodeURIComponent(userId)}/items`).then((r) => r.items),
  log: () => api<{ items: ModLogItem[] }>('/admin/moderation/log').then((r) => r.items),
  ban: (userId: string, reason: string) => post<{ ok: boolean }>(`/admin/moderation/users/${encodeURIComponent(userId)}/ban`, { reason }),
  unban: (userId: string, reason: string) => post<{ ok: boolean }>(`/admin/moderation/users/${encodeURIComponent(userId)}/unban`, { reason }),
  hide: (kind: ModKind, id: string, reason: string) => post<{ ok: boolean }>('/admin/moderation/hide', { kind, id, reason }),
  unhide: (kind: ModKind, id: string, reason: string) => post<{ ok: boolean }>('/admin/moderation/unhide', { kind, id, reason }),
}

export const retention = {
  recap: () => api<Recap>('/me/recap'),
  notifications: () => api<NotificationList>('/me/notifications'),
  markNotificationsRead: () => post<{ ok: boolean }>('/me/notifications/read'),
}

// Fair play: reports from users, the admin queue.
export const fairplay = {
  report: (handle: string, reason: string, details: string) =>
    post<{ ok: boolean }>('/reports', { target_kind: 'user', target: handle, reason, details }),
  overview: () => api<import('./types').FairOverview>('/admin/fairplay'),
  resolveFlags: (subjectId: string, status: 'dismissed' | 'actioned') =>
    post<{ ok: boolean }>('/admin/fairplay/flags/resolve', { subject_kind: 'submission', subject_id: subjectId, status }),
  resolveReport: (id: string, status: 'dismissed' | 'actioned') =>
    post<{ ok: boolean }>(`/admin/fairplay/reports/${encodeURIComponent(id)}/resolve`, { status }),
}
