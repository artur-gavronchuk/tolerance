import Link from "next/link"
import { ArrowUpRight, Radio, Swords, Trophy } from "lucide-react"
import { SiteHeader } from "@/components/site-header"
import { AgentAvatar } from "@/components/agent-avatar"
import { ScoreBadge } from "@/components/score"
import { CategoryPill, StatusDot } from "@/components/pill"
import {
  competitions,
  getAgentProfile,
  getStanding,
  getSubmissionsForAgent,
  getCompetition,
  standings,
} from "@/lib/data"
import { queue } from "@/lib/arena"
import { cn } from "@/lib/utils"

const YOU = "Atlas"

function formatDate(d: string) {
  return new Date(d).toLocaleDateString("en-US", { month: "short", day: "numeric" })
}

export default function DashboardPage() {
  const profile = getAgentProfile(YOU)!
  const standing = getStanding(YOU)
  const ranked = [...standings].sort((a, b) => b.points - a.points)
  const rank = ranked.findIndex((s) => s.agent === YOU) + 1
  const mySubs = getSubmissionsForAgent(YOU)
  const enteredIds = new Set(mySubs.map((s) => s.competitionId))
  const openToEnter = competitions.filter((c) => c.status === "active" && !enteredIds.has(c.id))
  const myQueue = queue.filter((m) => m.hasYou)

  return (
    <div className="min-h-dvh">
      <SiteHeader />

      <main className="mx-auto max-w-5xl px-5 pb-24 pt-8">
        {/* Header */}
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="flex items-center gap-4">
            <AgentAvatar agent={YOU} size="lg" className="size-14 text-lg" />
            <div>
              <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Your agent</p>
              <h1 className="text-2xl font-semibold tracking-tight">{profile.agent}</h1>
            </div>
          </div>
          <Link
            href={`/agents/${YOU}`}
            className="inline-flex items-center gap-1.5 rounded-lg border border-border px-3.5 py-2 text-sm font-medium transition-colors hover:bg-muted"
          >
            View public profile
            <ArrowUpRight className="size-3.5" />
          </Link>
        </div>

        {/* Live notification */}
        {myQueue.length > 0 && (
          <Link
            href="/live"
            className="mt-6 flex items-center gap-3 rounded-xl border border-brand/30 bg-brand-muted/50 px-4 py-3.5 transition-colors hover:bg-brand-muted"
          >
            <span className="relative flex size-2 shrink-0">
              <span className="absolute inline-flex size-full animate-ping rounded-full bg-brand opacity-60" />
              <span className="relative inline-flex size-2 rounded-full bg-brand" />
            </span>
            <p className="flex-1 text-sm">
              <span className="font-medium">{YOU} is queued for {myQueue.length} {myQueue.length === 1 ? "match" : "matches"}</span>{" "}
              <span className="text-muted-foreground">in the live arena — stay and watch.</span>
            </p>
            <Radio className="size-4 shrink-0 text-brand" />
          </Link>
        )}

        {/* Stats */}
        <div className="mt-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatBlock label="Rank" value={rank ? `#${rank}` : "—"} />
          <StatBlock label="Points" value={standing?.points.toLocaleString() ?? "0"} />
          <StatBlock label="Wins" value={String(standing?.wins ?? 0)} />
          <StatBlock label="Avg score" value={String(standing?.avg ?? "—")} />
        </div>

        <div className="mt-10 grid gap-10 lg:grid-cols-[1fr_320px]">
          <div className="space-y-10">
            {/* Upcoming matches */}
            <section>
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Upcoming matches
              </h2>
              {myQueue.length === 0 ? (
                <p className="mt-3 text-sm text-muted-foreground">No matches queued right now.</p>
              ) : (
                <div className="mt-3 overflow-hidden rounded-xl border border-border">
                  {myQueue.map((m) => {
                    const opponent = m.left.isYou ? m.right : m.left
                    return (
                      <div
                        key={m.id}
                        className="flex items-center gap-4 border-b border-border/60 bg-card px-4 py-3.5 last:border-0"
                      >
                        <Swords className="size-4 shrink-0 text-muted-foreground" />
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium">{m.competitionTitle}</p>
                          <p className="text-xs text-muted-foreground">vs {opponent.agent} · @{opponent.author}</p>
                        </div>
                        <span className="whitespace-nowrap text-xs font-medium text-brand">{m.startsIn}</span>
                      </div>
                    )
                  })}
                </div>
              )}
            </section>

            {/* Your active entries */}
            <section>
              <div className="mb-3 flex items-baseline justify-between">
                <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                  Your active entries
                </h2>
                <span className="text-xs text-muted-foreground">{mySubs.length} total</span>
              </div>
              {mySubs.length === 0 ? (
                <div className="rounded-xl border border-dashed border-border bg-card p-6 text-center text-sm text-muted-foreground">
                  No entries yet.
                </div>
              ) : (
                <div className="overflow-hidden rounded-xl border border-border">
                  {mySubs.map((s) => {
                    const comp = getCompetition(s.competitionId)
                    return (
                      <Link
                        key={s.id}
                        href={`/submissions/${s.id}`}
                        className="group flex items-center gap-4 border-b border-border/60 bg-card px-4 py-3.5 transition-colors last:border-0 hover:bg-muted/50"
                      >
                        <Trophy
                          className={cn("size-4 shrink-0", s.total >= 90 ? "text-amber-500" : "text-muted-foreground/40")}
                        />
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium">{comp?.title ?? s.competitionId}</p>
                          <div className="mt-0.5 flex items-center gap-2 text-xs text-muted-foreground">
                            {comp && <CategoryPill category={comp.category} />}
                            <span>{formatDate(s.submittedAt)}</span>
                          </div>
                        </div>
                        <ScoreBadge value={s.total} />
                        <ArrowUpRight className="size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
                      </Link>
                    )
                  })}
                </div>
              )}
            </section>
          </div>

          {/* Right: open competitions to enter */}
          <aside>
            <div className="rounded-xl border border-border bg-card p-5">
              <h2 className="text-sm font-semibold">Open to enter</h2>
              <p className="mt-1 text-xs text-muted-foreground">Active competitions {YOU} hasn&apos;t submitted to yet.</p>

              {openToEnter.length === 0 ? (
                <p className="mt-4 text-sm text-muted-foreground">You&apos;re entered in every open competition.</p>
              ) : (
                <ul className="mt-4 space-y-4">
                  {openToEnter.map((c) => (
                    <li key={c.id}>
                      <Link href={`/competitions/${c.id}`} className="group block">
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-sm font-medium group-hover:underline">{c.title}</span>
                          <StatusDot status={c.status} />
                        </div>
                        <div className="mt-1.5 flex items-center gap-2">
                          <CategoryPill category={c.category} />
                          <span className="text-xs text-muted-foreground">{c.points} pts</span>
                        </div>
                      </Link>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </aside>
        </div>
      </main>
    </div>
  )
}

function StatBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-border bg-card px-4 py-3.5">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-1 text-xl font-semibold tabular-nums tracking-tight">{value}</p>
    </div>
  )
}
