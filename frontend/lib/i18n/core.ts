// Tiny i18n core shared by server and client code. No library: messages are plain objects, one file per
// area under lib/i18n/messages/, each with an `en` and a `ru` table holding the same keys.
//
//   export const m = defineMessages({
//     en: { title: 'Today', closes: 'Closes in {time}', 'days.one': '{n} day', 'days.other': '{n} days' },
//     ru: { title: 'Сегодня', closes: 'Закроется через {time}', 'days.one': '{n} день', 'days.few': '{n} дня',
//           'days.many': '{n} дней', 'days.other': '{n} дня' },
//   })
//   const t = useT(m)            // client component
//   const t = await getT(m)      // server component / generateMetadata
//   t('closes', { time: '05:12' })
//   t.plural('days', 3)          // picks days.one/few/many/other by the locale's plural rules, {n} is filled in

export const LOCALES = ['en', 'ru'] as const
export type Locale = (typeof LOCALES)[number]
export const DEFAULT_LOCALE: Locale = 'en'
export const LOCALE_COOKIE = 'lang'
export const LOCALE_NAMES: Record<Locale, string> = { en: 'English', ru: 'Русский' }

export function isLocale(v: unknown): v is Locale {
  return typeof v === 'string' && (LOCALES as readonly string[]).includes(v)
}

// The saved choice wins; otherwise the first supported language in Accept-Language; otherwise English.
export function pickLocale(saved?: string | null, acceptLanguage?: string | null): Locale {
  if (isLocale(saved)) return saved
  for (const part of (acceptLanguage ?? '').split(',')) {
    const tag = part.split(';')[0].trim().toLowerCase().split('-')[0]
    if (isLocale(tag)) return tag
  }
  return DEFAULT_LOCALE
}

// BCP 47 tag for Intl formatting. en-GB keeps day-month order and a 24h clock.
export function intlLocale(locale: Locale) {
  return locale === 'ru' ? 'ru-RU' : 'en-GB'
}

type Dict = Record<string, string>
export type Messages<K extends string = string> = { en: Record<K, string>; ru: Partial<Record<string, string>> & Record<K, string> }

// Declares one area's messages. `ru` must have every key `en` has (plus optional extra plural forms like
// `x.few` / `x.many`), so a missing translation is a type error.
export function defineMessages<const E extends Dict>(m: { en: E; ru: { [K in keyof E]: string } & Dict }) {
  return m as { en: E; ru: { [K in keyof E]: string } & Dict }
}

export type Vars = Record<string, string | number>

function fill(s: string, vars?: Vars) {
  if (!vars) return s
  return s.replace(/\{(\w+)\}/g, (all, k: string) => (k in vars ? String(vars[k]) : all))
}

// Plural keys look like `base.one`; `PluralBase` recovers `base` so t.plural('days', n) type-checks.
type PluralBase<K> = K extends `${infer B}.${'one' | 'few' | 'many' | 'other'}` ? B : never

export type T<E extends Dict> = ((key: keyof E & string, vars?: Vars) => string) & {
  plural: (base: PluralBase<keyof E & string>, n: number, vars?: Vars) => string
  locale: Locale
}

export function makeT<E extends Dict>(m: { en: E; ru: Dict }, locale: Locale): T<E> {
  const table: Dict = locale === 'en' ? m.en : m.ru
  const rules = new Intl.PluralRules(intlLocale(locale))
  const get = (key: string) => table[key] ?? (m.en as Dict)[key] ?? key
  const t = ((key: string, vars?: Vars) => fill(get(key), vars)) as T<E>
  t.plural = (base, n, vars) => {
    const form = rules.select(n)
    const key = table[`${base}.${form}`] !== undefined ? `${base}.${form}` : `${base}.other`
    return fill(get(key), { n, ...vars })
  }
  t.locale = locale
  return t
}

// Dates and times in the reader's language. Times are UTC unless `local` is set, with the zone named.
export function formatDate(locale: Locale, iso: string, opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short', year: 'numeric' }) {
  return new Date(iso).toLocaleDateString(intlLocale(locale), { timeZone: 'UTC', ...opts })
}

export function formatDateTime(locale: Locale, iso: string, opts: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) {
  return `${new Date(iso).toLocaleString(intlLocale(locale), { timeZone: 'UTC', ...opts })} UTC`
}

export function formatNumber(locale: Locale, n: number, opts?: Intl.NumberFormatOptions) {
  return n.toLocaleString(intlLocale(locale), opts)
}
