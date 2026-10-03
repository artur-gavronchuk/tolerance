'use client'

import { useCallback, useEffect, useState } from 'react'
import { SectionTitle } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ApiError, moderation } from '@/lib/api'
import { errorText } from '@/lib/i18n/messages/errors'
import { useT } from '@/lib/i18n/client'
import { formatDateTime } from '@/lib/i18n/core'
import { moderationMessages } from '@/lib/i18n/messages/admin-moderation'
import type { ModItem, ModLogItem, ModUser } from '@/lib/types'
import { ReasonForm } from './reason-form'

export function ModerationSection() {
  const t = useT(moderationMessages)
  const [q, setQ] = useState('')
  const [users, setUsers] = useState<ModUser[] | null>(null)
  const [log, setLog] = useState<ModLogItem[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<ApiError | Error | null>(null)

  const search = useCallback(async (query: string) => {
    setBusy(true)
    try {
      setUsers(await moderation.users(query))
      setError(null)
    } catch (e) {
      setError(e as Error)
    } finally {
      setBusy(false)
    }
  }, [])
  const reloadLog = useCallback(() => moderation.log().then(setLog).catch((e) => setError(e as Error)), [])
  // Called after any action: the user list and the log both change.
  const changed = useCallback(() => { void search(q); void reloadLog() }, [q, search, reloadLog])

  useEffect(() => { void search(''); void reloadLog() }, [search, reloadLog])

  return (
    <section>
      <SectionTitle>{t('title')}</SectionTitle>
      <p className="-mt-1 mb-3 text-sm text-muted-foreground">{t('lead')}</p>
      <form onSubmit={(e) => { e.preventDefault(); void search(q) }} className="flex gap-2">
        <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('search')} aria-label={t('search')} className="min-w-0 flex-1" />
        <Button type="submit" disabled={busy}>{busy ? t('searching') : t('searchGo')}</Button>
      </form>
      {error && <p role="alert" className="mt-2 text-sm text-destructive">{errorText(error, t.locale)}</p>}
      <ul className="mt-3 divide-y divide-border rounded-xl border border-border bg-card">
        {users?.length === 0 && <li className="p-4 text-sm text-muted-foreground">{t('noUsers')}</li>}
        {users?.map((u) => <UserRow key={u.id} u={u} onChanged={changed} />)}
      </ul>
      {log && <ModLog items={log} />}
    </section>
  )
}

function UserRow({ u, onChanged }: { u: ModUser; onChanged: () => void }) {
  const t = useT(moderationMessages)
  const [form, setForm] = useState(false)
  const [items, setItems] = useState<ModItem[] | null>(null)
  const [showItems, setShowItems] = useState(false)
  const loadItems = useCallback(() => moderation.items(u.id).then(setItems), [u.id])
  return (
    <li className="min-w-0 space-y-2 p-3 sm:p-4">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <span className="font-semibold">{u.handle}</span>
        {u.banned_at && <Badge variant="destructive">{t('banned')}</Badge>}
        {u.role === 'admin' && <Badge variant="secondary">{t('admin')}</Badge>}
        <span className="min-w-0 break-all text-sm text-muted-foreground">{u.email}</span>
      </div>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-xs text-muted-foreground">
        <span>{formatDateTime(t.locale, u.created_at)}</span>
        <span>{t('activity', { s: u.submissions })}</span>
        <span className="ml-auto flex gap-2">
          <Button size="sm" variant="outline" onClick={() => { setShowItems(!showItems); if (!showItems) void loadItems() }}>{showItems ? t('close') : t('items')}</Button>
          {u.role !== 'admin' && !form && (
            <Button size="sm" variant={u.banned_at ? 'outline' : 'destructive'} onClick={() => setForm(true)}>{u.banned_at ? t('unban') : t('ban')}</Button>
          )}
        </span>
      </div>
      {form && (
        <ReasonForm label={u.banned_at ? t('unban') : t('ban')}
          run={(r) => (u.banned_at ? moderation.unban(u.id, r) : moderation.ban(u.id, r))}
          onDone={() => { setForm(false); onChanged(); if (showItems) void loadItems() }} onCancel={() => setForm(false)} />
      )}
      {showItems && items && (
        <ul className="divide-y divide-border rounded-lg border border-border">
          {items.length === 0 && <li className="p-3 text-sm text-muted-foreground">{t('noItems')}</li>}
          {items.map((it) => <ItemRow key={`${it.kind}-${it.id}`} it={it} onChanged={() => { void loadItems(); onChanged() }} />)}
        </ul>
      )}
    </li>
  )
}

function ItemRow({ it, onChanged }: { it: ModItem; onChanged: () => void }) {
  const t = useT(moderationMessages)
  const [form, setForm] = useState(false)
  return (
    <li className="space-y-2 p-3 text-sm">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <Badge variant="outline">{t(`kind.${it.kind}` as 'kind.bot')}</Badge>
        <span className="min-w-0 break-words font-semibold">{it.label}</span>
        <span className="text-xs text-muted-foreground">{it.status} · {formatDateTime(t.locale, it.at)}</span>
        {it.hidden_at && <Badge variant="destructive">{t('hidden')}</Badge>}
        {!form && (
          <Button size="xs" variant="outline" className="ml-auto" onClick={() => setForm(true)}>{it.hidden_at ? t('unhide') : t('hide')}</Button>
        )}
      </div>
      {form && (
        <ReasonForm label={it.hidden_at ? t('unhide') : t('hide')}
          run={(r) => (it.hidden_at ? moderation.unhide(it.kind, it.id, r) : moderation.hide(it.kind, it.id, r))}
          onDone={() => { setForm(false); onChanged() }} onCancel={() => setForm(false)} />
      )}
    </li>
  )
}

function ModLog({ items }: { items: ModLogItem[] }) {
  const t = useT(moderationMessages)
  return (
    <div className="mt-6">
      <SectionTitle aside={t('last50')}>{t('log')}</SectionTitle>
      <ul className="divide-y divide-border rounded-xl border border-border bg-card">
        {items.length === 0 && <li className="p-4 text-sm text-muted-foreground">{t('nothingYet')}</li>}
        {items.map((l, i) => (
          <li key={`${l.at}-${i}`} className="min-w-0 space-y-1 p-3 text-sm">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <Badge variant={l.action === 'ban' || l.action === 'hide' ? 'destructive' : 'outline'}>{t(`action.${l.action}` as 'action.ban')}</Badge>
              <span className="text-muted-foreground">{t(`kind.${l.kind}` as 'kind.bot')}</span>
              <span className="min-w-0 break-words font-semibold">{l.label || l.id}</span>
              {l.active && <span className="text-xs font-semibold text-destructive">{t('inForce')}</span>}
            </div>
            <p className="break-words text-muted-foreground">{l.reason}</p>
            <p className="text-xs text-muted-foreground">{l.actor} · {formatDateTime(t.locale, l.at)}</p>
          </li>
        ))}
      </ul>
    </div>
  )
}
