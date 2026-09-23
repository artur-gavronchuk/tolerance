import Link from "next/link"
import { ArrowUpRight, Users } from "lucide-react"
import type { Competition } from "@/lib/data"
import { CategoryPill, DifficultyPill, StatusDot } from "@/components/pill"

export function CompetitionCard({ competition }: { competition: Competition }) {
  return (
    <Link
      href={`/competitions/${competition.id}`}
      className="group flex flex-col rounded-xl border border-border bg-card p-5 transition-all hover:border-foreground/20 hover:shadow-sm"
    >
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <CategoryPill category={competition.category} />
          <DifficultyPill difficulty={competition.difficulty} />
        </div>
        <StatusDot status={competition.status} />
      </div>

      <h3 className="text-pretty text-base font-semibold leading-snug tracking-tight">
        {competition.title}
      </h3>
      <p className="mt-1.5 line-clamp-2 text-sm leading-relaxed text-muted-foreground">
        {competition.summary}
      </p>

      <div className="mt-4 flex items-center justify-between border-t border-border/70 pt-3.5">
        <div className="flex items-center gap-4 text-xs text-muted-foreground">
          <span className="font-medium text-foreground">{competition.points} pts</span>
          <span className="flex items-center gap-1">
            <Users className="size-3.5" />
            {competition.participants}
          </span>
        </div>
        <span className="flex items-center gap-0.5 text-xs font-medium text-muted-foreground transition-colors group-hover:text-foreground">
          View
          <ArrowUpRight className="size-3.5 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
        </span>
      </div>
    </Link>
  )
}
