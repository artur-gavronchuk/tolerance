'use client'

import { createContext, useCallback, useContext, useMemo } from 'react'
import { useRouter } from 'next/navigation'
import { DEFAULT_LOCALE, LOCALE_COOKIE, makeT, type Locale, type T } from './core'

const LocaleContext = createContext<Locale>(DEFAULT_LOCALE)

// Set once in the root layout from the cookie / Accept-Language the server saw.
export function I18nProvider({ locale, children }: { locale: Locale; children: React.ReactNode }) {
  return <LocaleContext.Provider value={locale}>{children}</LocaleContext.Provider>
}

export function useLocale() {
  return useContext(LocaleContext)
}

export function useT<E extends Record<string, string>>(m: { en: E; ru: Record<string, string> }): T<E> {
  const locale = useLocale()
  return useMemo(() => makeT(m, locale), [m, locale])
}

// Saves the choice for a year and re-renders server components in the new language.
export function useSetLocale() {
  const router = useRouter()
  return useCallback((locale: Locale) => {
    document.cookie = `${LOCALE_COOKIE}=${locale}; path=/; max-age=31536000; samesite=lax`
    router.refresh()
  }, [router])
}
