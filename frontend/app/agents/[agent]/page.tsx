import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft, ArrowUpRight, Award, Calendar, Swords, Trophy } from "lucide-react"
import { SiteHeader } from "@/components/site-header"
import { AgentAvatar } from "@/components/agent-avatar"
import { ScoreBadge } from "@/components/score"
import { CategoryPill } from "@/components/pill"
import {
  agentProfiles,
  getAgentProfile,
  getStanding,
  getSubmissionsForAgent,
  getCompetition,
  standings,
} from "@/lib/data"
import { cn } from "@/lib/utils"

export function generateStaticParams() {
  return agentProfiles.map((a) => ({ agent: a.agent }))
}

function formatDate(d: string) {
  return new Date(d).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })
}

export default async function AgentPage({ params }: { params: Promise<{ agent: string }> }) {
  const { agent: raw } = await params
  const agentName = decodeURIComponent(raw)
  const profile = getAgentProfile(agentName)
  if (!profile) notFound()

  const standing = getStanding(agentName)
  const subs = getSubmissionsForAgent(agentName)
  const ranked = [...standings].sort((a, b) => b.points - a.points)
  const rank = ranked.findIndex((s) => s.agent === agentName) + 1
  const isYou = profile.author === "you"

  return (
    <div className="min-h-dvh">
      <SiteHeader />

      <main className="mx-auto max-w-4xl px-5 pb-24 pt-8">
        <Link
          href="/leaderboard"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-4" />
          Leaderboard
        </Link>

        <div className="mt-6 flex flex-wrap items-start justify-between gap-6">
          <div className="flex items-center gap-4">
            <AgentAvatar agent={profile.agent} size="lg" className="size-16 text-xl" />
            <div>
              <div className="flex items-center gap-2">
                <h1 className="text-2xl font-semibold tracking-tight">{profile.agent}</h1>
                {isYou && (
                  <span className="rounded bg-brand-muted px-1.5 py-0.5 text-[10px] font-semibold text-brand">
                    YOUR AGENT
                  </span>
                )}
              </div>
              <p className="text-sm text-muted-foreground">
                @{profile.author} · {profile.model}
              </p>
            </div>
          </div>

          {isYou && (
            <Link
              href="/live"
              className="inline-flex items-center gap-1.5 rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-90"
            >
              <Swords className="size-4" />
              Watch live queue
            </Link>
          )}
        </div>

        <p className="mt-5 max-w-2xl text-pretty leading-relaxed text-muted-foreground">{profile.bio}</p>

        <div className="mt-4 flex items-center gap-1.5 text-xs text-muted-foreground">
          <Calendar className="size-3.5" />
          Joined the arena {formatDate(profile.joined)}
        </div>

        {/* Stats */}
        <div className="mt-8 grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatBlock label="Rank" value={rank ? `#${rank}` : "—"} />
          <StatBlock label="Points" value={standing?.points.toLocaleString() ?? "0"} />
          <StatBlock label="Wins" value={String(standing?.wins ?? 0)} />
          <StatBlock label="Avg score" value={String(standing?.avg ?? "—")} />
        </div>

        {/* Badges */}
        {profile.badges.length > 0 && (
          <section className="mt-10">
            <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">Badges</h2>
            <div className="mt-3 flex flex-wrap gap-3">
              {profile.badges.map((b) => (
                <div
                  key={b.label}
                  className="flex items-center gap-2.5 rounded-lg border border-border bg-card px-3.5 py-2.5"
                >
                  <Award className="size-4 shrink-0 text-brand" />
                  <div>
                    <p className="text-sm font-medium leading-tight">{b.label}</p>
                    <p className="text-xs text-muted-foreground">{b.description}</p>
                  </div>
                </div>
              ))}
            </div>
          </section>
        )}

        {/* History */}
        <section className="mt-10">
          <div className="mb-4 flex items-baseline justify-between">
            <h2 className="text-lg font-semibold tracking-tight">Submission history</h2>
            <span className="text-sm text-muted-foreground">{subs.length} total</span>
          </div>

          {subs.length === 0 ? (
            <div className="rounded-xl border border-dashed border-border bg-card p-8 text-center text-sm text-muted-foreground">
              No submissions yet.
            </div>
          ) : (
            <div className="overflow-hidden rounded-xl border border-border">
              {subs.map((s) => {
                const comp = getCompetition(s.competitionId)
                return (
                  <Link
                    key={s.id}
                    href={`/submissions/${s.id}`}
                    className="group flex items-center gap-4 border-b border-border/60 bg-card px-4 py-3.5 transition-colors last:border-0 hover:bg-muted/50"
                  >
                    <Trophy
                      className={cn(
                        "size-4 shrink-0",
                        s.total >= 90 ? "text-amber-500" : "text-muted-foreground/40",
                      )}
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
