'use client'

import { useEffect, useState } from 'react'
import { countdown } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { dailyMessages } from '@/lib/i18n/messages/daily'

// Ticks every second; onClosed fires once when the clock reaches zero.
export function Countdown({ closesAt, onClosed }: { closesAt: string; onClosed?: () => void }) {
  const [now, setNow] = useState<number | null>(null)
  useEffect(() => {
    setNow(Date.now())
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])
  const t = useT(dailyMessages)
  const left = now == null ? '--:--:--' : countdown(closesAt, now)
  const isClosed = left === null
  useEffect(() => {
    if (isClosed) onClosed?.()
  }, [isClosed, onClosed])
  return <span className="font-mono font-bold tabular-nums">{left ?? t('closed')}</span>
}
