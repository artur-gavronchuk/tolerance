// Server-side reads of the Go API for metadata and share images. The browser
// goes through the /api rewrite; the server talks to the API directly (API_URL,
// the same variable next.config.mjs uses for the rewrite).
// Returns null on any failure (API down, 404, bad JSON): callers fall back to
// the default card, a share preview must never break a page.
const API = process.env.API_URL ?? 'http://127.0.0.1:8080'

export async function serverApi<T>(path: string): Promise<T | null> {
  try {
    const res = await fetch(`${API}/api/v1${path}`, {
      headers: { Accept: 'application/json' },
      signal: AbortSignal.timeout(3000),
      next: { revalidate: 60 },
    })
    if (!res.ok) return null
    return (await res.json()) as T
  } catch {
    return null
  }
}

// Absolute site origin for metadataBase.
export const SITE_URL = (process.env.ARENA_PUBLIC_URL || process.env.NEXT_PUBLIC_SITE_URL || 'http://localhost:3000').replace(/\/+$/, '')
