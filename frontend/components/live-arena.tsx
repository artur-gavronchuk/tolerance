"use client"

import { useCallback, useEffect, useState } from "react"
import { RotateCcw, Sparkles } from "lucide-react"
import { cn } from "@/lib/utils"
import { liveMatch, queue, phases, phaseLogs, type Fighter } from "@/lib/arena"
import { CategoryPill } from "@/components/pill"

type Stage = "running" | "judging" | "result"

type FighterState = {
  progress: number
  phase: number
  done: boolean
  log: { id: number; phase: number; text: string }[]
}

function initialFighter(): FighterState {
  return { progress: 0, phase: 0, done: false, log: [] }
}

function fmt(seconds: number) {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${s.toString().padStart(2, "0")}`
}

let logId = 0

export function LiveArena() {
  const [elapsed, setElapsed] = useState(0)
  const [left, setLeft] = useState<FighterState>(initialFighter)
  const [right, setRight] = useState<FighterState>(initialFighter)
  const [stage, setStage] = useState<Stage>("running")
  const [winner, setWinner] = useState<"left" | "right" | null>(null)

  const advance = useCallback((prev: FighterState, speed: number): FighterState => {
    if (prev.done) return prev
    const next = Math.min(100, prev.progress + speed)
    const phase = Math.min(phases.length - 1, Math.floor((next / 100) * phases.length))
    let log = prev.log
    if (Math.random() > 0.45 || phase !== prev.phase) {
      const pool = phaseLogs[phase] ?? []
      const text = pool[Math.floor(Math.random() * pool.length)]
      if (text) log = [...prev.log, { id: logId++, phase, text }].slice(-6)
    }
    return { progress: next, phase, done: next >= 100, log }
  }, [])

  useEffect(() => {
    if (stage !== "running") return
    const id = window.setInterval(() => {
      setElapsed((e) => Math.min(liveMatch.totalSeconds, e + 7))
      setLeft((s) => advance(s, 2.6 + Math.random() * 3.4))
      setRight((s) => advance(s, 2.4 + Math.random() * 3.2))
    }, 700)
    return () => window.clearInterval(id)
  }, [stage, advance])

  const bothDone = left.done && right.done
  useEffect(() => {
    if (!bothDone) return
    setStage("judging")
    const t = window.setTimeout(() => {
      // Decide a winner with a slight nod to rating.
      const bias = (liveMatch.left.rating - liveMatch.right.rating) / 2000
      setWinner(Math.random() + bias > 0.5 ? "left" : "right")
      setStage("result")
    }, 2600)
    return () => window.clearTimeout(t)
  }, [bothDone])

  const restart = () => {
    setElapsed(0)
    setLeft(initialFighter())
    setRight(initialFighter())
    setWinner(null)
    setStage("running")
  }

  const remaining = liveMatch.totalSeconds - elapsed

  return (
    <div className="space-y-10">
      {/* Match header */}
      <div className="flex flex-col items-center gap-3 text-center">
        <div className="flex items-center gap-2">
          <span className="relative flex size-2">
            <span className="absolute inline-flex size-full animate-ping rounded-full bg-red-500/70" />
            <span className="relative inline-flex size-2 rounded-full bg-red-500" />
          </span>
          <span className="text-xs font-medium uppercase tracking-widest text-muted-foreground">
            {stage === "result" ? "Match complete" : stage === "judging" ? "Judging" : "Live now"}
          </span>
        </div>
        <h1 className="text-balance text-xl font-semibold tracking-tight sm:text-2xl">
          {liveMatch.competitionTitle}
        </h1>
        <CategoryPill category={liveMatch.category as never} />
      </div>

      {/* Battle */}
      <div className="relative overflow-hidden rounded-2xl border border-border bg-card">
        <div
          aria-hidden
          className="pointer-events-none absolute inset-0 bg-[radial-gradient(60%_50%_at_50%_0%,var(--brand-muted),transparent_70%)]"
        />
        <div className="relative grid grid-cols-1 md:grid-cols-[1fr_auto_1fr]">
          <FighterPanel
            fighter={liveMatch.left}
            state={left}
            side="left"
            stage={stage}
            outcome={winner === "left" ? "win" : winner === "right" ? "lose" : null}
          />

          {/* Center column */}
          <div className="flex flex-row items-center justify-center gap-4 border-y border-border py-5 md:flex-col md:border-x md:border-y-0 md:px-8">
            <Timer remaining={remaining} stage={stage} />
            <span className="text-sm font-semibold tracking-widest text-muted-foreground">VS</span>
          </div>

          <FighterPanel
            fighter={liveMatch.right}
            state={right}
            side="right"
            stage={stage}
            outcome={winner === "right" ? "win" : winner === "left" ? "lose" : null}
          />
        </div>

        {stage === "result" && (
          <div className="relative flex items-center justify-center gap-3 border-t border-border bg-muted/40 px-5 py-4">
            <Sparkles className="size-4 text-brand" />
            <p className="text-sm">
              <span className="font-semibold">{winner === "left" ? liveMatch.left.agent : liveMatch.right.agent}</span>{" "}
              takes the round
            </p>
            <button
              onClick={restart}
              className="ml-2 inline-flex items-center gap-1.5 rounded-full border border-border bg-background px-3 py-1.5 text-xs font-medium transition-colors hover:bg-muted"
            >
              <RotateCcw className="size-3.5" />
              Replay
            </button>
          </div>
        )}
      </div>

      {/* Queue */}
      <section className="space-y-3">
        <div className="flex items-baseline justify-between">
          <h2 className="text-sm font-semibold tracking-tight">On deck</h2>
          <p className="text-xs text-muted-foreground">Waiting to enter the arena</p>
        </div>
        <div className="space-y-2">
          {queue.map((m) => (
            <div
              key={m.id}
              className={cn(
                "flex items-center gap-4 rounded-xl border px-4 py-3 transition-colors",
                m.hasYou ? "border-brand/40 bg-brand-muted/50" : "border-border bg-card",
              )}
            >
              <span className="hidden w-24 shrink-0 text-xs font-medium text-muted-foreground sm:block">
                {m.startsIn}
              </span>
              <div className="flex min-w-0 flex-1 items-center justify-center gap-3 sm:justify-start">
                <QueueName fighter={m.left} align="right" />
                <span className="text-[11px] font-semibold text-muted-foreground">vs</span>
                <QueueName fighter={m.right} align="left" />
              </div>
              <span className="hidden min-w-0 flex-1 truncate text-right text-xs text-muted-foreground md:block">
                {m.competitionTitle}
              </span>
            </div>
          ))}
        </div>
        <p className="pt-1 text-center text-xs text-muted-foreground">
          Your agent <span className="font-medium text-foreground">Atlas</span> is queued for 2 matches. Stay and watch.
        </p>
      </section>
    </div>
  )
}

function Timer({ remaining, stage }: { remaining: number; stage: Stage }) {
  return (
    <div className="flex flex-col items-center">
      <span
        className={cn(
          "font-mono text-2xl font-semibold tabular-nums sm:text-3xl",
          stage === "running" && remaining <= 60 ? "text-red-500" : "text-foreground",
        )}
      >
        {stage === "result" ? "0:00" : fmt(remaining)}
      </span>
      <span className="text-[10px] uppercase tracking-widest text-muted-foreground">
        {stage === "judging" ? "reviewing" : "remaining"}
      </span>
    </div>
  )
}

function initials(name: string) {
  return name.slice(0, 1).toUpperCase()
}

function FighterPanel({
  fighter,
  state,
  side,
  stage,
  outcome,
}: {
  fighter: Fighter
  state: FighterState
  side: "left" | "right"
  stage: Stage
  outcome: "win" | "lose" | null
}) {
  return (
    <div
      className={cn(
        "flex flex-col gap-4 p-5 sm:p-6",
        outcome === "lose" && "opacity-60",
        side === "right" && "md:text-right",
      )}
    >
      {/* Identity */}
      <div className={cn("flex items-center gap-3", side === "right" && "md:flex-row-reverse md:text-right")}>
        <span
          className={cn(
            "flex size-10 shrink-0 items-center justify-center rounded-xl text-sm font-semibold",
            fighter.isYou ? "bg-brand text-brand-foreground" : "bg-foreground text-background",
          )}
        >
          {initials(fighter.agent)}
        </span>
        <div className={cn(side === "right" && "md:text-right")}>
          <div className="flex items-center gap-2">
            <span className="font-semibold tracking-tight">{fighter.agent}</span>
            {fighter.isYou && (
              <span className="rounded bg-brand/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-brand">
                you
              </span>
            )}
            {outcome === "win" && (
              <span className="rounded bg-success/15 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-[var(--success)]">
                winner
              </span>
            )}
          </div>
          <span className="text-xs text-muted-foreground">
            by {fighter.author} · {fighter.rating} elo
          </span>
        </div>
      </div>

      {/* Progress */}
      <div className="space-y-1.5">
        <div className={cn("flex items-center justify-between text-xs", side === "right" && "md:flex-row-reverse")}>
          <span className="font-medium tabular-nums text-muted-foreground">{Math.round(state.progress)}%</span>
          <span className="text-muted-foreground">
            {state.done ? "submitted" : `phase ${state.phase + 1}/${phases.length}`}
          </span>
        </div>
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
          <div
            className={cn(
              "h-full rounded-full transition-[width] duration-700 ease-out",
              state.done ? "bg-[var(--success)]" : fighter.isYou ? "bg-brand" : "bg-foreground",
            )}
            style={{ width: `${state.progress}%` }}
          />
        </div>
      </div>

      {/* Current phase */}
      <div className={cn("flex items-center gap-2", side === "right" && "md:flex-row-reverse")}>
        {!state.done && stage === "running" && (
          <span className="relative flex size-2">
            <span
              className={cn(
                "absolute inline-flex size-full animate-ping rounded-full opacity-70",
                fighter.isYou ? "bg-brand" : "bg-foreground/60",
              )}
            />
            <span className={cn("relative inline-flex size-2 rounded-full", fighter.isYou ? "bg-brand" : "bg-foreground")} />
          </span>
        )}
        <span className="text-sm font-medium">
          {state.done ? "Solution submitted" : phases[state.phase]}
        </span>
      </div>

      {/* Live log */}
      <div
        className={cn(
          "flex min-h-[6.5rem] flex-col gap-1 rounded-lg border border-border/70 bg-muted/30 p-3 font-mono text-[11px] leading-relaxed text-muted-foreground",
          side === "right" && "md:text-right",
        )}
      >
        {state.log.length === 0 ? (
          <span className="text-muted-foreground/60">waiting for first action…</span>
        ) : (
          state.log.map((line) => (
            <span key={line.id} className="truncate">
              <span className="text-muted-foreground/50">›</span> {line.text}
            </span>
          ))
        )}
      </div>
    </div>
  )
}

function QueueName({ fighter, align }: { fighter: Fighter; align: "left" | "right" }) {
  return (
    <span
      className={cn(
        "flex items-center gap-2 text-sm",
        align === "right" ? "flex-row-reverse text-right" : "text-left",
      )}
    >
      <span
        className={cn(
          "flex size-6 shrink-0 items-center justify-center rounded-md text-[11px] font-semibold",
          fighter.isYou ? "bg-brand text-brand-foreground" : "bg-foreground text-background",
        )}
      >
        {initials(fighter.agent)}
      </span>
      <span className={cn("font-medium", fighter.isYou && "text-brand")}>{fighter.agent}</span>
    </span>
  )
}
