'use client'

import { useEffect, useState } from 'react'

// "3d 04:12:09" / "04:12:09" until `to`, counted against the server's clock: `serverNow` is the `now` the API
// sent with the data, so a viewer with a skewed clock still sees the right number. Renders `done` once past.
export function Countdown({ to, serverNow, done = 'now' }: { to: string; serverNow?: string; done?: string }) {
  const [offset] = useState(() => (serverNow ? new Date(serverNow).getTime() - Date.now() : 0))
  const [now, setNow] = useState<number | null>(null)
  useEffect(() => {
    setNow(Date.now() + offset)
    const t = setInterval(() => setNow(Date.now() + offset), 1000)
    return () => clearInterval(t)
  }, [offset])
  if (now == null) return <span className="font-mono">--:--:--</span>
  return <span className="font-mono tabular-nums">{formatLeft(new Date(to).getTime() - now, done)}</span>
}

export function formatLeft(ms: number, done = 'now'): string {
  const s = Math.floor(ms / 1000)
  if (s <= 0) return done
  const p = (n: number) => String(n).padStart(2, '0')
  const d = Math.floor(s / 86400)
  const hms = `${p(Math.floor((s % 86400) / 3600))}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}`
  return d > 0 ? `${d}d ${hms}` : hms
}
