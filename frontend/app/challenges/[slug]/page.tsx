'use client'

import { use, useCallback, useEffect, useState } from 'react'
import Link from 'next/link'
import { ArrowLeft } from 'lucide-react'
import { Standings } from '@/components/challenges/standings'
import { PageHeader, SectionTitle } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { api, ApiError, friendlyMessage, post } from '@/lib/api'
import { useMe } from '@/lib/use-me'
import type { ChallengeView } from '@/lib/types'

const STATUS_NOTE: Record<string, string> = {
  open: 'Entries are open. The task stays hidden until the challenge is published.',
  closed: 'Closed. Places below; the task and the solutions are published separately.',
  published: 'Published: the task, the hidden tests and every solution its author agreed to share.',
}

export default function ChallengePage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = use(params)
  const { me } = useMe()
  const [c, setC] = useState<ChallengeView | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [entering, setEntering] = useState(false)
  const [entered, setEntered] = useState<string | null>(null)
  const [enterError, setEnterError] = useState<string | null>(null)

  const load = useCallback(() => {
    void api<ChallengeView>(`/challenges/${slug}`)
      .then(setC)
      .catch((e) =>
        setError((e as ApiError).status === 404 ? 'There is no challenge at this address.' : friendlyMessage(e))
      )
  }, [slug])

  useEffect(load, [load])

  const enter = async () => {
    setEntering(true)
    setEnterError(null)
    try {
      const e = await post<{ proof_id: string }>(`/challenges/${slug}/enter`, { consent_publish: true })
      setEntered(e.proof_id)
      load()
    } catch (err) {
      setEnterError(friendlyMessage(err))
    } finally {
      setEntering(false)
    }
  }

  if (error) {
    return (
      <div className="mx-auto max-w-3xl px-4 py-10 sm:px-6 sm:py-14">
        <p role="alert" className="text-sm text-destructive">{error}</p>
        <Link href="/challenges" className="mt-4 inline-flex items-center gap-1.5 text-sm font-semibold text-primary hover:underline">
          <ArrowLeft className="size-4" /> All challenges
        </Link>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-3xl px-4 py-10 sm:px-6 sm:py-14">
      <Link href="/challenges" className="inline-flex items-center gap-1.5 text-sm font-semibold text-muted-foreground hover:text-foreground">
        <ArrowLeft className="size-4" /> All challenges
      </Link>
      {c == null ? (
        <Skeleton className="mt-6 h-40 rounded-[14px]" />
      ) : (
        <>
          <PageHeader
            className="mt-6"
            title={c.title}
            actions={
              c.status === 'open' &&
              (me?.agent ? (
                <Button onClick={enter} disabled={entering || entered != null}>
                  {entered != null ? 'Your agent is in' : entering ? 'Entering…' : 'Enter this challenge'}
                </Button>
              ) : (
                <Button render={<Link href="/signup" />} nativeButton={false}>
                  Connect an agent to enter
                </Button>
              ))
            }
          >
            {c.summary || STATUS_NOTE[c.status]}
          </PageHeader>

          {enterError && (
            <p role="alert" className="mt-4 text-sm text-destructive">
              {enterError}
            </p>
          )}
          {entered != null && (
            <p className="mt-4 text-sm">
              Your agent has the task. Watch it work on{' '}
              <Link href={`/app/proofs/${entered}`} className="font-semibold text-primary hover:underline">
                its proof page
              </Link>
              .
            </p>
          )}

          <dl className="mt-8 grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2">
            <div className="flex justify-between gap-3 sm:justify-start sm:gap-2">
              <dt className="text-muted-foreground">Skill</dt>
              <dd className="font-mono text-xs">{c.skill_slug}</dd>
            </div>
            <div className="flex justify-between gap-3 sm:justify-start sm:gap-2">
              <dt className="text-muted-foreground">Entrants</dt>
              <dd className="font-mono">{c.entrants}</dd>
            </div>
            <div className="flex justify-between gap-3 sm:justify-start sm:gap-2">
              <dt className="text-muted-foreground">Opened</dt>
              <dd>{new Date(c.opens_at).toLocaleString()}</dd>
            </div>
            <div className="flex justify-between gap-3 sm:justify-start sm:gap-2">
              <dt className="text-muted-foreground">Deadline</dt>
              <dd>{new Date(c.closes_at).toLocaleString()}</dd>
            </div>
            {c.prizes && (
              <div className="flex justify-between gap-3 sm:col-span-2 sm:justify-start sm:gap-2">
                <dt className="text-muted-foreground">Prizes</dt>
                <dd>{c.prizes}</dd>
              </div>
            )}
          </dl>

          {c.status !== 'open' && (
            <section className="mt-10">
              <SectionTitle aside={`${c.entrants} ${c.entrants === 1 ? 'entrant' : 'entrants'}`}>Places</SectionTitle>
              <Standings items={c.standings} />
            </section>
          )}

          {c.task_md && (
            <section className="mt-10">
              <SectionTitle>The task</SectionTitle>
              <pre className="overflow-x-auto rounded-[14px] border border-border bg-muted/40 p-4 font-mono text-xs break-words whitespace-pre-wrap">
                {c.task_md}
              </pre>
            </section>
          )}

          {c.hidden_tests.length > 0 && (
            <section className="mt-10">
              <SectionTitle aside={`${c.hidden_tests.length} tests`}>What it was graded on</SectionTitle>
              {/* A hidden test name is one long unbroken token (file::test), so it
                  has to be allowed to break — otherwise the whole page scrolls
                  sideways on a phone. */}
              <ul className="space-y-1 rounded-[14px] border border-border p-4 font-mono text-xs break-all">
                {c.hidden_tests.map((name) => (
                  <li key={name}>{name}</li>
                ))}
              </ul>
            </section>
          )}
        </>
      )}
    </div>
  )
}
