'use client'

import { createContext, useContext, useEffect, useState } from 'react'
import Link from 'next/link'
import { Bot, Medal, Package, Trophy } from 'lucide-react'
import { PhaseBadge } from '@/components/products/phase'
import { SectionTitle } from '@/components/page-header'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { api, friendlyMessage, products } from '@/lib/api'
import type { Activity, ActivityBot, ActivityProduct, ProductList } from '@/lib/types'

// One fetch of /users/{handle}/activity shared by the stack chip (above the daily header) and the sections below it.
type State = { data: Activity | null; error: string | null }
const Ctx = createContext<State>({ data: null, error: null })

export function ActivityProvider({ handle, children }: { handle: string; children: React.ReactNode }) {
  const [state, setState] = useState<State>({ data: null, error: null })
  useEffect(() => {
    let live = true
    api<Activity>(`/users/${encodeURIComponent(handle)}/activity`)
      .then((data) => live && setState({ data, error: null }))
      .catch((e) => live && setState({ data: null, error: friendlyMessage(e) }))
    return () => { live = false }
  }, [handle])
  return <Ctx.Provider value={state}>{children}</Ctx.Provider>
}

export function MainStack() {
  const { data } = useContext(Ctx)
  if (!data?.stack) return null
  const s = data.stack
  return (
    <p className="mb-3 flex flex-wrap items-center gap-2 text-sm text-muted-foreground" title={`Most used across ${s.count} upload${s.count === 1 ? '' : 's'}`}>
      Main stack
      <Badge variant="secondary" className="h-6 px-2.5 text-sm font-semibold">{s.label}</Badge>
    </p>
  )
}

function Empty({ children }: { children: React.ReactNode }) {
  return <p className="rounded-[14px] border border-dashed border-input px-5 py-8 text-center text-sm text-muted-foreground">{children}</p>
}

const MEDAL = ['text-warning', 'text-muted-foreground', 'text-[#b4784a]']

function PlaceBadge({ place, of }: { place: number; of: number }) {
  return (
    <span className="inline-flex shrink-0 items-center gap-1 rounded-4xl border border-border px-2.5 py-0.5 text-xs font-bold">
      {place <= 3 ? <Medal className={`size-3.5 ${MEDAL[place - 1]}`} /> : null}
      #{place}<span className="font-normal text-muted-foreground">of {of}</span>
    </span>
  )
}

function ProductCard({ p }: { p: ActivityProduct }) {
  const href = p.phase === 'open' ? `/products/${p.task_slug}` : `/products/${p.task_slug}/results`
  const checks = p.total > 0 ? `${p.passed}/${p.total} checks` : null
  return (
    <li>
      <Link href={href} className="flex h-full flex-col gap-3 rounded-[14px] border border-border bg-card p-4 transition-colors hover:border-primary/50">
        <div className="flex items-start justify-between gap-2">
          <span className="min-w-0 break-words font-semibold">{p.task_title}</span>
          {p.place != null && <PlaceBadge place={p.place} of={p.entrants} />}
        </div>
        <div className="mt-auto flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <PhaseBadge phase={p.phase} />
          <Badge variant="outline">{p.kind}</Badge>
          {p.phase === 'open' ? (
            <span>Results after {p.deadline.slice(0, 10)}</span>
          ) : (
            <>
              {checks && <span className="font-mono">{checks}</span>}
              <span className="font-mono">{p.votes} vote{p.votes === 1 ? '' : 's'}</span>
            </>
          )}
        </div>
      </Link>
    </li>
  )
}

// The call to action depends on whether a product task is open right now.
function NoProducts() {
  const [list, setList] = useState<ProductList | null>(null)
  useEffect(() => {
    let live = true
    products.list().then((l) => live && setList(l)).catch(() => {})
    return () => { live = false }
  }, [])
  const open = list?.items.find((t) => t.phase === 'open')
  const next = list?.upcoming?.next_opens_at
  return (
    <Empty>
      <Package className="mx-auto mb-2 size-5" />
      No product tasks yet
      {open ? <> — <Link href={`/products/${open.slug}`} className="font-semibold text-primary hover:underline">enter this week&apos;s task</Link></>
        : next ? <> — next product task opens {new Date(next).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })}</>
        : null}
    </Empty>
  )
}

export function ProductsSection() {
  const { data } = useContext(Ctx)
  return (
    <section>
      <SectionTitle>Products</SectionTitle>
      {!data ? <Skeleton className="h-24 rounded-[14px]" /> : data.products.length === 0 ? (
        <NoProducts />
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2">{data.products.map((p) => <ProductCard key={p.task_slug} p={p} />)}</ul>
      )}
    </section>
  )
}

function BotCard({ b }: { b: ActivityBot }) {
  const titles = b.tournaments.filter((t) => t.champion)
  return (
    <li className="flex h-full flex-col gap-3 rounded-[14px] border border-border bg-card p-4">
      <div className="flex items-start justify-between gap-2">
        <Link href={`/tanks/bots/${b.id}`} className="min-w-0 break-words font-semibold hover:text-primary hover:underline">{b.name}</Link>
        <span className="shrink-0 font-mono text-lg font-bold">{b.rating}</span>
      </div>
      <p className="text-xs text-muted-foreground">
        {b.rank != null ? <>#{b.rank} this season</> : 'Not on the ladder'} · {b.matches} match{b.matches === 1 ? '' : 'es'}, {b.wins} win{b.wins === 1 ? '' : 's'}
        <br />
        Lifetime {b.lifetime_rating} · {b.total_matches} matches, {b.total_wins} wins
      </p>
      {(titles.length > 0 || b.best_finish) && (
        <div className="mt-auto flex flex-wrap items-center gap-2 text-xs">
          {titles.map((t) => (
            <Link key={t.id} href={`/tanks/tournaments/${t.id}`} title={t.name}
              className="inline-flex max-w-full items-center gap-1 rounded-4xl border border-warning bg-warning/10 px-2.5 py-0.5 font-bold hover:text-primary">
              <Trophy className="size-3.5 shrink-0 text-warning" /><span className="truncate">{t.name}</span>
            </Link>
          ))}
          {titles.length === 0 && b.best_finish && <span className="text-muted-foreground">Best tournament finish: <b className="text-foreground">{b.best_finish}</b></span>}
        </div>
      )}
    </li>
  )
}

export function TanksSection() {
  const { data } = useContext(Ctx)
  return (
    <section>
      <SectionTitle>Tanks</SectionTitle>
      {!data ? <Skeleton className="h-24 rounded-[14px]" /> : data.bots.length === 0 ? (
        <Empty>
          <Bot className="mx-auto mb-2 size-5" />
          No bots yet — <Link href="/app/tanks" className="font-semibold text-primary hover:underline">build one</Link>
        </Empty>
      ) : (
        <ul className="grid gap-3 sm:grid-cols-2">{data.bots.map((b) => <BotCard key={b.id} b={b} />)}</ul>
      )}
    </section>
  )
}

export function ActivityError() {
  const { error } = useContext(Ctx)
  return error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null
}
