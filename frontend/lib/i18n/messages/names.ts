import { defineMessages, formatDate, formatDateTime, intlLocale, makeT, type Locale } from '../core'
import { tanksHomeMessages } from './tanks-home'

// Names the server generates in English (tournaments, seasons, results) are rebuilt here from their structured
// fields, so they follow the reader's language.
export const namesMessages = defineMessages({
  en: {
    weekly: 'Weekly tournament, {when}',
    open: 'Open tournament, {when}',
  },
  ru: {
    weekly: 'Еженедельный турнир, {when}',
    open: 'Открытый турнир, {when}',
  },
})

// "October 2026" / "Октябрь 2026". Seasons are calendar months in UTC.
export function seasonName(locale: Locale, startsAt: string) {
  const month = new Date(startsAt).toLocaleDateString(intlLocale(locale), { timeZone: 'UTC', month: 'long' })
  const year = new Date(startsAt).getUTCFullYear()
  return `${month.charAt(0).toUpperCase()}${month.slice(1)} ${year}`
}

// Season ids are "YYYY-MM".
export const seasonNameFromId = (locale: Locale, id: string) => seasonName(locale, `${id}-01T00:00:00Z`)

// Weekly: "Weekly tournament, 3 Oct". Open (on demand): "Open tournament, 3 Oct, 08:41 UTC".
export function tournamentName(locale: Locale, tour: { open: boolean; starts_at: string; name: string }) {
  if (!tour.starts_at) return tour.name
  const t = makeT(namesMessages, locale)
  if (tour.open) return t('open', { when: formatDateTime(locale, tour.starts_at, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit', hour12: false }) })
  return t('weekly', { when: formatDate(locale, tour.starts_at, { day: 'numeric', month: 'short' }) })
}

// How far a bot got in a tournament ("Champion", "Out in round 2", ...).
export function resultLabel(locale: Locale, r: string) {
  const t = makeT(tanksHomeMessages, locale)
  if (r === 'Champion' || r === 'In progress' || r === 'Runner-up' || r === 'Semifinalist' || r === 'Quarterfinalist') return t(`result.${r}`)
  const out = /^Out in round (\d+)$/.exec(r)
  return out ? t('result.out', { n: out[1] }) : r
}
