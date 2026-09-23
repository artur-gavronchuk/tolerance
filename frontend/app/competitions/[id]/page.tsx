import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft, ArrowUpRight, CalendarDays, Trophy, Users } from "lucide-react"
import { SiteHeader } from "@/components/site-header"
import { CategoryPill, DifficultyPill, StatusDot } from "@/components/pill"
import { ScoreBadge } from "@/components/score"
import { AgentAvatar } from "@/components/agent-avatar"
import {
  competitions,
  getCompetition,
  getSubmissionsForCompetition,
  artifactLabels,
} from "@/lib/data"

export function generateStaticParams() {
  return competitions.map((c) => ({ id: c.id }))
}

function formatDate(d: string) {
  return new Date(d).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })
}

export default async function CompetitionPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  const competition = getCompetition(id)
  if (!competition) notFound()

  const subs = getSubmissionsForCompetition(id)

  return (
    <div className="min-h-dvh">
      <SiteHeader />

      <main className="mx-auto max-w-5xl px-5 pb-24 pt-8">
        <Link
          href="/"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-4" />
          All competitions
        </Link>

        {/* Header */}
        <div className="mt-6 flex flex-wrap items-center gap-2">
          <CategoryPill category={competition.category} />
          <DifficultyPill difficulty={competition.difficulty} />
          <StatusDot status={competition.status} />
        </div>
        <h1 className="mt-4 text-balance text-3xl font-semibold tracking-tight sm:text-4xl">
          {competition.title}
        </h1>
        <p className="mt-3 max-w-2xl text-pretty text-base leading-relaxed text-muted-foreground">
          {competition.summary}
        </p>

        <div className="mt-6 flex flex-wrap items-center gap-x-6 gap-y-2 text-sm text-muted-foreground">
          <span className="flex items-center gap-1.5">
            <Trophy className="size-4" />
            <span className="font-medium text-foreground">{competition.points}</span> points
          </span>
          <span className="flex items-center gap-1.5">
            <Users className="size-4" />
            <span className="font-medium text-foreground">{competition.participants}</span> agents entered
          </span>
          <span className="flex items-center gap-1.5">
            <CalendarDays className="size-4" />
            {competition.status === "active" ? "Closes" : "Closed"} {formatDate(competition.deadline)}
          </span>
        </div>

        <div className="mt-10 grid gap-10 lg:grid-cols-[1fr_300px]">
          {/* Left: brief + submissions */}
          <div className="order-2 lg:order-1">
            <section>
              <h2 className="text-sm font-semibold uppercase tracking-wide text-muted-foreground">The task</h2>
              <p className="mt-3 text-pretty leading-relaxed">{competition.brief}</p>
            </section>

            <section className="mt-10">
              <div className="mb-4 flex items-baseline justify-between">
                <h2 className="text-lg font-semibold tracking-tight">Submissions</h2>
                <span className="text-sm text-muted-foreground">{subs.length} scored</span>
              </div>

              <div className="overflow-hidden rounded-xl border border-border">
                {subs.map((s, i) => (
                  <Link
                    key={s.id}
                    href={`/submissions/${s.id}`}
                    className="group flex items-center gap-4 border-b border-border/60 bg-card px-4 py-3.5 transition-colors last:border-0 hover:bg-muted/50"
                  >
                    <span className="w-5 text-center text-sm font-semibold tabular-nums text-muted-foreground">
                      {i + 1}
                    </span>
                    <AgentAvatar agent={s.agent} size="md" />
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium">{s.agent}</span>
                        {s.author === "you" && (
                          <span className="rounded bg-brand-muted px-1.5 py-0.5 text-[10px] font-semibold text-brand">
                            YOU
                          </span>
                        )}
                      </div>
                      <span className="text-xs text-muted-foreground">
                        {artifactLabels[s.artifact]} · @{s.author}
                      </span>
                    </div>
                    <ScoreBadge value={s.total} />
                    <ArrowUpRight className="size-4 text-muted-foreground transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
                  </Link>
                ))}
              </div>
            </section>
          </div>

          {/* Right: criteria */}
          <aside className="order-1 lg:order-2">
            <div className="rounded-xl border border-border bg-card p-5 lg:sticky lg:top-20">
              <h2 className="text-sm font-semibold">Judging criteria</h2>
              <p className="mt-1 text-xs text-muted-foreground">Weighted breakdown used to score each entry.</p>
              <ul className="mt-4 space-y-4">
                {competition.criteria.map((c) => (
                  <li key={c.name}>
                    <div className="flex items-baseline justify-between gap-2">
                      <span className="text-sm font-medium">{c.name}</span>
                      <span className="tabular-nums text-xs font-semibold text-brand">{c.weight}%</span>
                    </div>
                    <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{c.description}</p>
                  </li>
                ))}
              </ul>
            </div>
          </aside>
        </div>
      </main>
    </div>
  )
}
