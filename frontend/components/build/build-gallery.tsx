'use client'

import { useState } from 'react'
import Link from 'next/link'
import { BadgeCheck, Bot, Heart } from 'lucide-react'
import { useLoginHref } from '@/components/public/return-path'
import { api } from '@/lib/api'
import { errorText } from '@/lib/format'
import { useT } from '@/lib/i18n/client'
import { buildMessages } from '@/lib/i18n/messages/build'
import type { BuildChallenge, BuildEntry } from '@/lib/types'
import { useMyHandle } from '@/lib/use-signed-in'
import { cn } from '@/lib/utils'

export const siteUrl = (e: BuildEntry) => `/api/v1/build-entries/${encodeURIComponent(e.id)}/site/index.html`
export const shotUrl = (e: BuildEntry) => `/api/v1/build-entries/${encodeURIComponent(e.id)}/shot.jpg?v=${e.version}`
export const entryHref = (slug: string, e: { id: string }) => `/c/${encodeURIComponent(slug)}/${encodeURIComponent(e.id)}`

type Sort = 'score' | 'votes'

// The ranked entries of a challenge (final score order from the server), the platform's reference first.
// The page's tab names the list, so there is no heading here.
export function BuildGallery({ challenge, entries, reference, onChange }: {
  challenge: BuildChallenge
  entries: BuildEntry[]
  reference: BuildEntry | null
  onChange: (e: BuildEntry) => void
}) {
  const t = useT(buildMessages)
  const [sort, setSort] = useState<Sort>('score')
  const sorted = sort === 'score' ? entries : [...entries].sort((a, b) => b.votes - a.votes || (b.score ?? 0) - (a.score ?? 0))
  const mobile = challenge.format === 'mobile'
  const canVote = challenge.status === 'open'

  return (
    <section>
      {entries.length > 1 && (
        <div className="mb-4 flex justify-end">
          <div className="flex rounded-full border border-border bg-card p-1 text-sm font-semibold">
            {(['score', 'votes'] as const).map((s) => (
              <button key={s} onClick={() => setSort(s)} aria-pressed={sort === s}
                className={cn('rounded-full px-3 py-1 transition-colors', sort === s ? 'bg-ink text-ink-foreground' : 'text-muted-foreground hover:text-foreground')}>
                {s === 'score' ? t('sortScore') : t('sortVotes')}
              </button>
            ))}
          </div>
        </div>
      )}
      <ul className={cn('grid gap-5', mobile ? 'grid-cols-2 sm:grid-cols-3 lg:grid-cols-4' : 'sm:grid-cols-2 lg:grid-cols-3')}>
        {reference && (
          <li><EntryCard e={reference} href={entryHref(challenge.slug, reference)} mobile={mobile} canVote={false} onChange={onChange} /></li>
        )}
        {sorted.map((e) => (
          <li key={e.id}><EntryCard e={e} href={entryHref(challenge.slug, e)} mobile={mobile} canVote={canVote} onChange={onChange} /></li>
        ))}
      </ul>
      {entries.length === 0 && (
        <p className="mt-5 rounded-[14px] border-2 border-dashed border-strong px-6 py-10 text-center text-muted-foreground">{t('galleryEmpty')}</p>
      )}
    </section>
  )
}

function EntryCard({ e, href, mobile, canVote, onChange }: { e: BuildEntry; href: string; mobile: boolean; canVote: boolean; onChange: (e: BuildEntry) => void }) {
  const t = useT(buildMessages)
  return (
    <article className={cn('group overflow-hidden rounded-[20px] border-2 bg-card',
      e.mine ? 'border-primary' : e.reference ? 'border-dashed border-strong' : 'border-transparent')}>
      <Link href={href} className={cn('relative block w-full overflow-hidden bg-muted', mobile ? 'aspect-[390/640]' : 'aspect-[16/10]')}
        aria-label={`${t('open')}: ${e.reference ? t('reference') : e.handle}`}>
        <Thumb e={e} />
        {e.reference ? (
          <span className="absolute left-3 top-3 flex items-center gap-1 rounded-full bg-terminal px-2.5 py-1 text-xs font-bold text-terminal-foreground">
            <BadgeCheck className="size-3.5" />{t('reference')}
          </span>
        ) : e.place > 0 && (
          <span className={cn('poster absolute left-3 top-3 flex h-9 min-w-9 items-center justify-center rounded-full px-2 text-base text-terminal',
            e.place === 1 ? 'bg-pop-6' : 'bg-card')}>{e.place}</span>
        )}
      </Link>
      <div className="space-y-3 p-4">
        <div className="flex items-center gap-2 sm:gap-3">
          <Link href={href} className="min-w-0 flex-1 hover:text-primary">
            <p className="truncate font-bold">{e.reference ? t('reference') : e.handle}</p>
            <p className="flex items-center gap-1 truncate text-xs text-muted-foreground">
              <Bot className="size-3 shrink-0" />{e.reference ? 'tolerance' : e.made_with || t('agentUnknown')}
            </p>
          </Link>
          <ScoreBadge score={e.reference ? e.test_score : e.score} max={e.reference ? 60 : 100} />
          {!e.reference && <VoteButton e={e} canVote={canVote} onChange={onChange} />}
        </div>
        {e.status === 'done' && <ScoreSplit tests={e.test_score ?? 0} votes={e.reference ? null : e.vote_score} />}
      </div>
    </article>
  )
}

export function Thumb({ e }: { e: BuildEntry }) {
  if (e.has_shot) {
    // eslint-disable-next-line @next/next/no-img-element
    return <img src={shotUrl(e)} alt="" loading="lazy" className="size-full object-cover object-top transition-transform duration-300 group-hover:scale-[1.03]" />
  }
  // No screenshot (fake sandbox locally): a live, inert, scaled-down frame.
  return (
    <iframe src={siteUrl(e)} title={e.handle} sandbox="allow-scripts" loading="lazy" tabIndex={-1}
      className="pointer-events-none h-[250%] w-[250%] origin-top-left scale-[0.4] border-0 bg-white" />
  )
}

// ScoreBadge is the final score out of 100 (or the test points of the reference).
export function ScoreBadge({ score, max = 100, large }: { score: number | null; max?: number; large?: boolean }) {
  if (score === null) return null
  const tone = score >= 0.8 * max ? 'bg-success text-success-foreground' : score >= 50 ? 'bg-warning text-warning-foreground' : 'bg-destructive/15 text-destructive'
  return (
    <span title="/100" className={cn('shrink-0 rounded-full font-mono font-bold tabular-nums', tone, large ? 'px-3.5 py-1 text-2xl' : 'px-2.5 py-0.5 text-sm')}>
      {score}
    </span>
  )
}

// ScoreBreakdown shows the two parts of the score and, apart, the quality hints that are not scored.
export function ScoreBreakdown({ e }: { e: BuildEntry }) {
  const t = useT(buildMessages)
  const hints = e.checks.points ?? {}
  return (
    <div className="space-y-4">
      <ScoreSplit tests={e.test_score ?? 0} votes={e.reference ? null : e.vote_score} labels />
      <p className="text-sm text-muted-foreground">
        {e.checks.total ? t('testsDetail', { passed: e.checks.passed ?? 0, total: e.checks.total }) : null}
        {!e.reference && <>{e.checks.total ? ', ' : ''}{t.plural('votes', e.votes)}</>}
      </p>
      {Object.keys(hints).length > 0 && (
        <div>
          <p className="text-sm font-semibold text-muted-foreground">{t('qualityTitle')}</p>
          <div className="mt-1.5 flex flex-wrap gap-1.5 text-xs">
            {(['a11y', 'mobile', 'perf'] as const).filter((k) => hints[k] !== undefined).map((k) => (
              <span key={k} className="rounded-full bg-muted px-2.5 py-1 font-semibold">
                {t(k === 'a11y' ? 'ptsA11y' : k === 'mobile' ? 'ptsMobile' : 'ptsPerf')} <span className="font-mono">{hints[k]}/10</span>
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  )
}

// ScoreSplit is the score's shape, everywhere the same: 60 points of hidden tests next to 40 points of votes.
// legend draws the empty rule (the home page), otherwise each part fills up to what the entry earned;
// votes = null leaves the votes part out of the picture (the reference takes no votes).
export function ScoreSplit({ tests, votes, legend, labels }: { tests?: number; votes?: number | null; legend?: boolean; labels?: boolean }) {
  const t = useT(buildMessages)
  const parts = [
    { key: 'tests', max: 60, v: legend ? 60 : tests ?? 0, fill: 'bg-ink', label: t('splitTests') },
    { key: 'votes', max: 40, v: legend ? 40 : votes ?? 0, fill: 'bg-pop-2', label: t('splitVotes'), off: votes === null && !legend },
  ]
  return (
    <div className="flex gap-1.5">
      {parts.map((p) => (
        <div key={p.key} style={{ flexGrow: p.max }} className={cn('basis-0', p.off && 'opacity-40')}>
          {legend && (
            <p className="mb-1.5 flex items-baseline gap-1.5">
              <span className="poster text-[2rem]">{p.max}</span>
              <span className="text-sm font-semibold text-muted-foreground">{p.label}</span>
            </p>
          )}
          {labels && (
            <p className="mb-1.5">
              <span className="poster block text-xl tabular-nums">{p.off ? '—' : p.v}<span className="text-sm text-muted-foreground">/{p.max}</span></span>
              <span className="text-sm font-semibold text-muted-foreground">{p.label}</span>
            </p>
          )}
          <div className={cn('overflow-hidden rounded-full bg-muted', legend ? 'h-3' : 'h-1.5')}>
            <div className={cn('h-full rounded-full', p.fill)} style={{ width: `${(100 * Math.min(p.v, p.max)) / p.max}%` }} />
          </div>
        </div>
      ))}
    </div>
  )
}

export function VoteButton({ e, canVote, onChange, big }: { e: BuildEntry; canVote: boolean; onChange: (e: BuildEntry) => void; big?: boolean }) {
  const t = useT(buildMessages)
  const handle = useMyHandle()
  const loginHref = useLoginHref()
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const label = e.mine ? t('ownEntry') : e.voted ? t('voted') : t('vote')
  const cls = cn('flex shrink-0 items-center gap-1.5 rounded-full border font-bold tabular-nums transition-colors disabled:opacity-60',
    big ? 'h-11 px-4' : 'h-9 px-3 text-sm',
    e.voted ? 'border-transparent bg-pop-2 text-white' : 'border-border bg-card text-foreground hover:border-pop-2 hover:text-pop-2')
  const inner = <><Heart className={cn('size-4', e.voted && 'fill-current')} />{e.votes}{big && <span className="ml-1">{label}</span>}</>

  if (!handle && canVote) {
    return <Link href={loginHref} className={cls} title={t('signInToVote')} aria-label={t('signInToVote')}>{inner}</Link>
  }
  async function toggle() {
    setBusy(true)
    setError(null)
    try {
      onChange(await api<BuildEntry>(`/build-entries/${encodeURIComponent(e.id)}/vote`, { method: e.voted ? 'DELETE' : 'POST' }))
    } catch (err) {
      setError(errorText(err, t.locale))
    } finally {
      setBusy(false)
    }
  }
  return (
    <button onClick={() => void toggle()} disabled={busy || e.mine || !canVote} aria-pressed={e.voted}
      title={error ?? label} aria-label={`${label}. ${t('votesLabel', { n: e.votes })}`} className={cls}>{inner}</button>
  )
}
