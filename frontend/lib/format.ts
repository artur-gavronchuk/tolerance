import { makeT, type Locale } from '@/lib/i18n/core'
import { dailyMessages } from '@/lib/i18n/messages/daily'

type DailyKey = keyof typeof dailyMessages.en

export function ago(iso: string, now = Date.now(), locale: Locale = 'en') {
  const t = makeT(dailyMessages, locale)
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000))
  if (s < 60) return t('ago.s', { n: s })
  if (s < 3600) return t('ago.m', { n: Math.round(s / 60) })
  if (s < 86400) return t('ago.h', { n: Math.round(s / 3600) })
  return t('ago.d', { n: Math.round(s / 86400) })
}

// Why a submission failed, in words. Keys are the backend's failure_reason values.
export function reasonLabel(reason: string | null, locale: Locale = 'en') {
  if (!reason) return null
  const key = `reason.${reason}`
  return key in dailyMessages.en ? makeT(dailyMessages, locale)(key as DailyKey) : reason.replace(/_/g, ' ')
}

export function statusLabel(status: string, locale: Locale = 'en') {
  const key = `status.${status}`
  return key in dailyMessages.en ? makeT(dailyMessages, locale)(key as DailyKey) : status
}

export function difficultyLabel(difficulty: number, locale: Locale = 'en') {
  const t = makeT(dailyMessages, locale)
  const key = `difficulty.${difficulty}`
  return key in dailyMessages.en ? t(key as DailyKey) : t('difficulty.other', { n: difficulty })
}

// "05:12:09" until `iso`, or null once it has passed.
export function countdown(iso: string, now = Date.now()) {
  const s = Math.floor((new Date(iso).getTime() - now) / 1000)
  if (s <= 0) return null
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(Math.floor(s / 3600))}:${p(Math.floor((s % 3600) / 60))}:${p(s % 60)}`
}

export { errorText } from '@/lib/i18n/messages/errors'
