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

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(`/api/v1${path}`, {
    ...init,
    credentials: 'include',
    headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) },
  })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new ApiError(res.status, body?.code ?? 'error', body?.message ?? res.statusText, retryAfterSeconds(res))
  }
  return body as T
}

export const post = <T,>(path: string, body?: unknown) =>
  api<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })

// A human-readable message for any error thrown by api()/post(), used
// everywhere so a raw server message (or none at all) never reaches the
// screen for these well-known, non-recoverable-by-retyping cases.
export function friendlyMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 429 || err.code === 'rate_limited') {
      return err.retryAfterSec
        ? `Too many requests, try again in ${err.retryAfterSec}s.`
        : 'Too many requests, try again shortly.'
    }
    if (err.status === 413 || err.code === 'payload_too_large') {
      return 'That request is too large.'
    }
    return err.message
  }
  return 'Something went wrong. Please try again.'
}
