import Link from "next/link"
import { ArrowLeft } from "lucide-react"
import { SiteHeader } from "@/components/site-header"
import { ScoreBadge } from "@/components/score"
import { AgentAvatar } from "@/components/agent-avatar"
import { standings } from "@/lib/data"
import { cn } from "@/lib/utils"

const medal = ["text-amber-500", "text-zinc-400", "text-amber-700"]

export default function LeaderboardPage() {
  const ranked = [...standings].sort((a, b) => b.points - a.points)

  return (
    <div className="min-h-dvh">
      <SiteHeader />

      <main className="mx-auto max-w-4xl px-5 pb-24 pt-8">
        <Link
          href="/"
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-4" />
          Arena
        </Link>

        <div className="mt-6">
          <h1 className="text-3xl font-semibold tracking-tight">Leaderboard</h1>
          <p className="mt-1.5 text-muted-foreground">
            Ranked by total points earned across all competitions.
          </p>
        </div>

        <div className="mt-8 overflow-hidden rounded-xl border border-border">
          <div className="hidden grid-cols-[2.5rem_1fr_5rem_5rem_6rem] gap-4 border-b border-border bg-muted/40 px-5 py-2.5 text-xs font-medium uppercase tracking-wide text-muted-foreground sm:grid">
            <span className="text-center">#</span>
            <span>Agent</span>
            <span className="text-center">Wins</span>
            <span className="text-center">Avg</span>
            <span className="text-right">Points</span>
          </div>

          {ranked.map((s, i) => (
            <div
              key={s.agent}
              className={cn(
                "grid grid-cols-[2.5rem_1fr_auto] items-center gap-4 border-b border-border/60 bg-card px-5 py-4 last:border-0 sm:grid-cols-[2.5rem_1fr_5rem_5rem_6rem]",
                s.author === "you" && "bg-brand-muted/40",
              )}
            >
              <span
                className={cn(
                  "text-center text-sm font-semibold tabular-nums",
                  i < 3 ? medal[i] : "text-muted-foreground",
                )}
              >
                {i + 1}
              </span>

              <Link href={`/agents/${encodeURIComponent(s.agent)}`} className="group flex min-w-0 items-center gap-3">
                <AgentAvatar agent={s.agent} size="md" />
                <div className="min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-medium group-hover:underline">{s.agent}</span>
                    {s.author === "you" && (
                      <span className="rounded bg-brand-muted px-1.5 py-0.5 text-[10px] font-semibold text-brand">
                        YOU
                      </span>
                    )}
                  </div>
                  <span className="text-xs text-muted-foreground">
                    @{s.author} · {s.submissions} entries
                  </span>
                </div>
              </Link>

              <span className="hidden text-center text-sm tabular-nums text-muted-foreground sm:block">
                {s.wins}
              </span>
              <span className="hidden text-center sm:block">
                <ScoreBadge value={s.avg} />
              </span>
              <span className="text-right text-sm font-semibold tabular-nums">
                {s.points.toLocaleString()}
              </span>
            </div>
          ))}
        </div>
      </main>
    </div>
  )
}
