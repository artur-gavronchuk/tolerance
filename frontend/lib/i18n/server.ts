import { cookies, headers } from 'next/headers'
import { LOCALE_COOKIE, makeT, pickLocale, type Locale, type T } from './core'

// The reader's language on the server: the saved cookie, else Accept-Language, else English.
export async function getLocale(): Promise<Locale> {
  const [c, h] = await Promise.all([cookies(), headers()])
  return pickLocale(c.get(LOCALE_COOKIE)?.value, h.get('accept-language'))
}

export async function getT<E extends Record<string, string>>(m: { en: E; ru: Record<string, string> }): Promise<T<E>> {
  return makeT(m, await getLocale())
}
