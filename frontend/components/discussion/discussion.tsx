'use client'

import { useCallback, useEffect, useState } from 'react'
import { HandleLink } from '@/components/daily/handle-link'
import { StackName } from '@/components/daily/stack-label'
import { CommentBody } from '@/components/discussion/comment-body'
import { SectionTitle } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { api, post } from '@/lib/api'
import { errorText } from '@/lib/format'
import { formatDateTime } from '@/lib/i18n/core'
import { useT } from '@/lib/i18n/client'
import { discussionMessages } from '@/lib/i18n/messages/discussion'
import { useMe } from '@/lib/use-me'

interface Comment {
  id: string
  handle: string
  made_with: string
  passed_tests: number | null
  total_tests: number | null
  body: string
  created_at: string
  edited_at: string | null
  mine: boolean
  can_edit: boolean
}

const MAX = 4000
const PAGE = 100

function Editor({ initial, submitLabel, onSubmit, onCancel }: { initial: string; submitLabel: string; onSubmit: (body: string) => Promise<void>; onCancel?: () => void }) {
  const t = useT(discussionMessages)
  const [body, setBody] = useState(initial)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const submit = async () => {
    setBusy(true)
    setError(null)
    try {
      await onSubmit(body)
      if (!onCancel) setBody('')
    } catch (e) {
      setError(errorText(e, t.locale))
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="space-y-2">
      <textarea value={body} onChange={(e) => setBody(e.target.value)} maxLength={MAX} rows={4} placeholder={t('placeholder')}
        className="w-full rounded-[10px] border border-input bg-background p-3 font-mono text-sm" />
      <div className="flex flex-wrap items-center gap-3">
        <Button onClick={() => void submit()} disabled={busy || !body.trim()}>{busy ? t('posting') : submitLabel}</Button>
        {onCancel && <button type="button" onClick={onCancel} className="text-sm font-semibold text-muted-foreground hover:text-foreground">{t('cancel')}</button>}
        <span className="text-xs text-muted-foreground">{t('chars', { n: body.length, max: MAX })}</span>
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    </div>
  )
}

function Item({ c, admin, reload }: { c: Comment; admin: boolean; reload: () => Promise<void> }) {
  const t = useT(discussionMessages)
  const [editing, setEditing] = useState(false)
  const [hiding, setHiding] = useState(false)
  const [reason, setReason] = useState('')
  const [error, setError] = useState<string | null>(null)
  const run = async (fn: () => Promise<unknown>) => {
    setError(null)
    try {
      await fn()
      await reload()
    } catch (e) {
      setError(errorText(e, t.locale))
    }
  }
  const linkBtn = 'text-xs font-semibold text-muted-foreground hover:text-foreground'
  return (
    <li className="rounded-[14px] border border-border p-4">
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-sm">
        <HandleLink handle={c.handle} className="font-bold" />
        {(c.made_with || c.total_tests != null) && (
          <span className="text-xs text-muted-foreground">
            {c.made_with && <StackName madeWith={c.made_with} />}
            {c.made_with && c.total_tests != null && ' · '}
            {c.total_tests != null && `${c.passed_tests}/${c.total_tests}`}
          </span>
        )}
        <span className="text-xs text-muted-foreground">
          {formatDateTime(t.locale, c.created_at)}{c.edited_at && ` · ${t('edited')}`}
        </span>
      </div>
      <div className="mt-2">
        {editing ? (
          <Editor initial={c.body} submitLabel={t('save')} onCancel={() => setEditing(false)}
            onSubmit={async (body) => { await api(`/comments/${c.id}`, { method: 'PATCH', body: JSON.stringify({ body }) }); setEditing(false); await reload() }} />
        ) : <CommentBody>{c.body}</CommentBody>}
      </div>
      {!editing && (c.can_edit || admin) && (
        <div className="mt-2 flex flex-wrap gap-3">
          {c.can_edit && <button type="button" className={linkBtn} onClick={() => setEditing(true)}>{t('edit')}</button>}
          {c.can_edit && <button type="button" className={linkBtn} onClick={() => void run(() => api(`/comments/${c.id}`, { method: 'DELETE' }))}>{t('delete')}</button>}
          {admin && !c.mine && <button type="button" className={linkBtn} onClick={() => setHiding((v) => !v)}>{t('hide')}</button>}
        </div>
      )}
      {hiding && (
        <div className="mt-2 flex flex-wrap gap-2">
          <input value={reason} onChange={(e) => setReason(e.target.value)} placeholder={t('hideReason')} maxLength={500}
            className="min-w-0 flex-1 rounded-[10px] border border-input bg-background px-3 py-1.5 text-sm" />
          <Button size="sm" disabled={!reason.trim()} onClick={() => void run(() => post('/admin/moderation/hide', { kind: 'comment', id: c.id, reason }))}>{t('hideConfirm')}</Button>
        </div>
      )}
      {error && <p role="alert" className="mt-2 text-sm text-destructive">{error}</p>}
    </li>
  )
}

// The thread under a closed day. The API refuses to serve it while the day is open.
export function Discussion({ day }: { day: string }) {
  const t = useT(discussionMessages)
  const { me, loading } = useMe()
  const [items, setItems] = useState<Comment[] | null>(null)
  const [total, setTotal] = useState(0)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async (limit = PAGE) => {
    try {
      const r = await api<{ items: Comment[]; total: number }>(`/daily/${day}/comments?limit=${limit}`)
      setItems(r.items)
      setTotal(r.total)
      setError(null)
    } catch (e) {
      setError(errorText(e, t.locale))
    }
  }, [day, t.locale])
  // Reload keeps whatever is already on screen (at least one page).
  const reload = useCallback(() => load(Math.max(PAGE, items?.length ?? 0)), [load, items])
  useEffect(() => { void load() }, [load])

  const more = async () => {
    try {
      const r = await api<{ items: Comment[]; total: number }>(`/daily/${day}/comments?offset=${items?.length ?? 0}&limit=${PAGE}`)
      setItems((cur) => [...(cur ?? []), ...r.items])
      setTotal(r.total)
    } catch (e) {
      setError(errorText(e, t.locale))
    }
  }

  return (
    <section>
      <SectionTitle aside={items ? t('count', { n: total }) : undefined}>{t('title')}</SectionTitle>
      <p className="mb-3 text-sm text-muted-foreground">{t('lead')}</p>
      {!loading && (me ? (
        <div className="mb-4">
          <Editor initial="" submitLabel={t('post')} onSubmit={async (body) => { await post(`/daily/${day}/comments`, { body }); await reload() }} />
        </div>
      ) : <p className="mb-4 text-sm font-semibold">{t('signIn')}</p>)}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {items && items.length === 0 && <p className="rounded-[14px] border border-dashed border-strong p-6 text-sm text-muted-foreground">{t('empty')}</p>}
      {items && items.length > 0 && (
        <ul className="space-y-3">{items.map((c) => <Item key={c.id} c={c} admin={!!me?.can_admin} reload={reload} />)}</ul>
      )}
      {items && total > items.length && (
        <button type="button" onClick={() => void more()} className="mt-3 rounded-full border border-input px-4 py-1.5 text-sm font-semibold hover:bg-muted">{t('showMore')}</button>
      )}
    </section>
  )
}
