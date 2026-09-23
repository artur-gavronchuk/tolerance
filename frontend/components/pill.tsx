import { cn } from "@/lib/utils"
import type { Category, Difficulty } from "@/lib/data"

const categoryStyles: Record<Category, string> = {
  "Full build": "text-brand",
  "Bug fix": "text-amber-600",
  "DB design": "text-violet-600",
  Refactor: "text-cyan-700",
  Integration: "text-rose-600",
}

const difficultyStyles: Record<Difficulty, string> = {
  Easy: "text-emerald-600",
  Medium: "text-amber-600",
  Hard: "text-rose-600",
}

export function Pill({ children, className }: { children: React.ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border border-border bg-card px-2.5 py-0.5 text-xs font-medium",
        className,
      )}
    >
      {children}
    </span>
  )
}

export function CategoryPill({ category }: { category: Category }) {
  return (
    <Pill>
      <span className={cn("size-1.5 rounded-full bg-current", categoryStyles[category])} />
      {category}
    </Pill>
  )
}

export function DifficultyPill({ difficulty }: { difficulty: Difficulty }) {
  return <Pill className={difficultyStyles[difficulty]}>{difficulty}</Pill>
}

export function StatusDot({ status }: { status: "active" | "past" }) {
  if (status === "active") {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-success">
        <span className="relative flex size-1.5">
          <span className="absolute inline-flex size-full animate-ping rounded-full bg-success opacity-60" />
          <span className="relative inline-flex size-1.5 rounded-full bg-success" />
        </span>
        Open
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
      <span className="size-1.5 rounded-full bg-muted-foreground/50" />
      Closed
    </span>
  )
}
