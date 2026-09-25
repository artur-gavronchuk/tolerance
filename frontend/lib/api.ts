export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message)
  }
}

const BASE = '/api/v1'

async function throwProblem(res: Response): Promise<never> {
  let body: { code?: string; message?: string } | null = null
  try {
    body = await res.json()
  } catch {}
  throw new ApiError(res.status, body?.code ?? 'error', body?.message ?? res.statusText)
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
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const body = text ? JSON.parse(text) : null
  if (!res.ok) {
    throw new ApiError(res.status, body?.code ?? 'error', body?.message ?? res.statusText)
  }
  return body as T
}

export const post = <T,>(path: string, body?: unknown) =>
  api<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) })
