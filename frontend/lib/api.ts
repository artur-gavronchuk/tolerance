import type {
  ProductDetail, ProductEntry, ProductList, ProductResults, ProductSourceFile, ProductTask, SeasonDetail, SeasonView, Showcase, StackRow, TournamentView,
} from './types'

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string, public retryAfterSec?: number) {
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
  let body: { code?: string; message?: string } | null = null
  try {
    body = await res.json()
  } catch {}
  throw new ApiError(res.status, body?.code ?? 'error', body?.message ?? res.statusText, retryAfterSeconds(res))
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

// A human-readable message for any error thrown by api()/post(), used
// everywhere so a raw server message (or none at all) never reaches the
// screen for these well-known, non-recoverable-by-retyping cases.
export function friendlyMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code !== 'attempts_exhausted' && (err.status === 429 || err.code === 'rate_limited')) {
      return err.retryAfterSec
        ? `Too many requests, try again in ${err.retryAfterSec}s.`
        : 'Too many requests, try again shortly.'
    }
    if (err.status === 413 || err.code === 'payload_too_large') {
      return 'That request is too large.'
    }
    if (err.code === 'attempts_exhausted') return 'No attempts left today for this task.'
    return err.message
  }
  return 'Something went wrong. Please try again.'
}

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
  results: (slug: string) => api<ProductResults>(`/products/${encodeURIComponent(slug)}/results`),
  submit: (slug: string, form: FormData) => upload<ProductEntry>(`/products/${encodeURIComponent(slug)}/entries`, form),
  vote: (entryId: string) => post<ProductEntry>(`/product-entries/${encodeURIComponent(entryId)}/vote`),
  unvote: (entryId: string) => api<ProductEntry>(`/product-entries/${encodeURIComponent(entryId)}/vote`, { method: 'DELETE' }),
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
