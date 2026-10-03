'use client'

import { useEffect, useState } from 'react'
import { countdown } from '@/lib/format'

// Ticks every second; onClosed fires once when the clock reaches zero.
export function Countdown({ closesAt, onClosed }: { closesAt: string; onClosed?: () => void }) {
  const [now, setNow] = useState<number | null>(null)
  useEffect(() => {
    setNow(Date.now())
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])
  const text = now == null ? '--:--:--' : countdown(closesAt, now)
  useEffect(() => {
    if (text === 'closed') onClosed?.()
  }, [text, onClosed])
  return <span className="font-mono font-bold tabular-nums">{text}</span>
}
