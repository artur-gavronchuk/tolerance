"use client"

import { useState } from "react"
import { Search, X } from "lucide-react"
import type { Competition, Category } from "@/lib/data"
import { CompetitionCard } from "@/components/competition-card"
import { cn } from "@/lib/utils"

type Tab = "active" | "past"
const categories: (Category | "All")[] = ["All", "Full build", "Bug fix", "DB design", "Refactor", "Integration"]

export function CompetitionBrowser({ competitions }: { competitions: Competition[] }) {
  const [tab, setTab] = useState<Tab>("active")
  const [category, setCategory] = useState<Category | "All">("All")
  const [query, setQuery] = useState("")

  const q = query.trim().toLowerCase()
  const filtered = competitions.filter(
    (c) =>
      c.status === tab &&
      (category === "All" || c.category === category) &&
      (q === "" || c.title.toLowerCase().includes(q) || c.summary.toLowerCase().includes(q)),
  )
  const activeCount = competitions.filter((c) => c.status === "active").length
  const pastCount = competitions.filter((c) => c.status === "past").length

  return (
    <section>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="inline-flex rounded-lg border border-border bg-muted/50 p-0.5">
          {(
            [
              { key: "active" as const, label: "Current", count: activeCount },
              { key: "past" as const, label: "Past", count: pastCount },
            ]
          ).map((t) => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={cn(
                "flex items-center gap-1.5 rounded-md px-3.5 py-1.5 text-sm font-medium transition-colors",
                tab === t.key ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground",
              )}
            >
              {t.label}
              <span
                className={cn(
                  "rounded px-1.5 py-0.5 text-[11px] tabular-nums",
                  tab === t.key ? "bg-muted text-foreground" : "text-muted-foreground",
                )}
              >
                {t.count}
              </span>
            </button>
          ))}
        </div>

        <div className="relative w-full sm:w-56">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search competitions"
            className="w-full rounded-lg border border-border bg-card py-1.5 pl-8 pr-8 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-foreground/30"
          />
          {query && (
            <button
              onClick={() => setQuery("")}
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
              aria-label="Clear search"
            >
              <X className="size-3.5" />
            </button>
          )}
        </div>
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-1.5">
        {categories.map((c) => (
          <button
            key={c}
            onClick={() => setCategory(c)}
            className={cn(
              "rounded-full border px-3 py-1 text-xs font-medium transition-colors",
              category === c
                ? "border-foreground bg-foreground text-background"
                : "border-border text-muted-foreground hover:border-foreground/30 hover:text-foreground",
            )}
          >
            {c}
          </button>
        ))}
      </div>

      {filtered.length > 0 ? (
        <div className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {filtered.map((c) => (
            <CompetitionCard key={c.id} competition={c} />
          ))}
        </div>
      ) : (
        <div className="mt-6 rounded-xl border border-dashed border-border py-16 text-center text-sm text-muted-foreground">
          {q ? (
            <>No competitions match &ldquo;{query}&rdquo;.</>
          ) : (
            <>No {tab === "active" ? "open" : "past"} competitions in this category yet.</>
          )}
        </div>
      )}
    </section>
  )
}
