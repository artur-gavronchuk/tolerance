'use client'

import { useParams, usePathname } from 'next/navigation'
import { useEffect } from 'react'
import { entrySource, flush, routePattern, track } from '@/lib/analytics'

// Sends one page_view per navigation, with the route pattern. Rendered once in the root layout.
export function AnalyticsProvider() {
  const pathname = usePathname()
  const params = useParams()
  useEffect(() => {
    const src = entrySource()
    track({ name: 'page_view', path: routePattern(pathname, params), ...(src ? { ref: src.ref, utm: src.utm, entry: true } : {}) })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname])
  useEffect(() => {
    const onHide = () => document.visibilityState === 'hidden' && flush()
    document.addEventListener('visibilitychange', onHide)
    return () => document.removeEventListener('visibilitychange', onHide)
  }, [])
  return null
}
