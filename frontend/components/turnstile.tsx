'use client'

import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'

// Inlined at build time, same pattern as NEXT_PUBLIC_CONTACT_EMAIL on
// /terms: see frontend/Dockerfile. Empty in dev, so the widget renders
// nothing and the signup form sends no extra field.
const SITE_KEY = process.env.NEXT_PUBLIC_TURNSTILE_SITE_KEY

export function turnstileEnabled(): boolean {
  return Boolean(SITE_KEY)
}

const SCRIPT_SRC = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'

interface TurnstileRenderOptions {
  sitekey: string
  theme: 'auto'
  size: 'flexible'
  callback: (token: string) => void
  'error-callback': () => void
  'expired-callback': () => void
}

declare global {
  interface Window {
    turnstile?: {
      render: (container: HTMLElement, options: TurnstileRenderOptions) => string
      reset: (widgetId: string) => void
      remove: (widgetId: string) => void
    }
  }
}

let scriptLoad: Promise<void> | null = null

function loadScript(): Promise<void> {
  if (window.turnstile) return Promise.resolve()
  if (!scriptLoad) {
    scriptLoad = new Promise((resolve, reject) => {
      const existing = document.querySelector<HTMLScriptElement>(`script[src="${SCRIPT_SRC}"]`)
      if (existing) {
        existing.addEventListener('load', () => resolve())
        existing.addEventListener('error', () => reject(new Error('turnstile script failed to load')))
        return
      }
      const script = document.createElement('script')
      script.src = SCRIPT_SRC
      script.async = true
      script.defer = true
      script.onload = () => resolve()
      script.onerror = () => reject(new Error('turnstile script failed to load'))
      document.head.appendChild(script)
    })
  }
  return scriptLoad
}

export interface TurnstileHandle {
  reset: () => void
}

// Explicit-render Cloudflare Turnstile widget. Renders nothing when no site
// key is configured (dev). `onToken(null)` on error/expiry disables submit
// again; the parent calls `reset()` after a failed submit since tokens are
// single-use.
export const Turnstile = forwardRef<TurnstileHandle, { onToken: (token: string | null) => void }>(
  function Turnstile({ onToken }, ref) {
    const containerRef = useRef<HTMLDivElement>(null)
    const widgetId = useRef<string | null>(null)

    useImperativeHandle(ref, () => ({
      reset: () => {
        if (widgetId.current && window.turnstile) window.turnstile.reset(widgetId.current)
      },
    }))

    useEffect(() => {
      if (!SITE_KEY || !containerRef.current) return
      let cancelled = false
      const container = containerRef.current
      loadScript()
        .then(() => {
          if (cancelled || !window.turnstile) return
          widgetId.current = window.turnstile.render(container, {
            sitekey: SITE_KEY,
            theme: 'auto',
            size: 'flexible',
            callback: (token) => onToken(token),
            'error-callback': () => onToken(null),
            'expired-callback': () => onToken(null),
          })
        })
        .catch(() => onToken(null))
      return () => {
        cancelled = true
        if (widgetId.current && window.turnstile) window.turnstile.remove(widgetId.current)
      }
      // Runs once: the widget is rendered a single time and reset imperatively.
    }, [])

    if (!SITE_KEY) return null
    return <div ref={containerRef} className="w-full" />
  },
)
