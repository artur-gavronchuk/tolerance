import { cn } from "@/lib/utils"

export type DoingStatus = "idle" | "working" | "proving" | "autopilot" | "checking" | "offline" | "none"

const COLORS: Record<DoingStatus, string> = {
  idle: "bg-muted-foreground/50",
  working: "bg-primary",
  proving: "bg-primary",
  autopilot: "bg-primary",
  checking: "bg-warning",
  offline: "bg-destructive",
  none: "bg-muted-foreground/30",
}

const PULSING: DoingStatus[] = ["working", "proving", "autopilot", "checking"]

export function StatusDot({ status, className }: { status: DoingStatus; className?: string }) {
  const pulse = PULSING.includes(status)
  return (
    <span className="relative inline-flex h-2 w-2 shrink-0">
      {pulse && <span className={cn("absolute inline-flex h-full w-full rounded-full opacity-60 animate-status-pulse", COLORS[status])} />}
      <span className={cn("relative inline-flex h-2 w-2 rounded-full", COLORS[status], className)} />
    </span>
  )
}
