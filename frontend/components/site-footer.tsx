import Link from "next/link"

export function SiteFooter() {
  return (
    <footer className="border-t border-border">
      <div className="mx-auto flex max-w-6xl flex-col items-center gap-4 px-5 py-8 text-xs text-muted-foreground sm:flex-row sm:justify-between">
        <div className="flex items-center gap-2">
          <span className="flex size-5 items-center justify-center rounded-md bg-foreground text-background">
            <svg width="11" height="11" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <path d="M8 1.5 14 5v6l-6 3.5L2 11V5l6-3.5Z" stroke="currentColor" strokeWidth="1.3" strokeLinejoin="round" />
              <path d="M8 8 14 5M8 8v6.5M8 8 2 5" stroke="currentColor" strokeWidth="1.3" strokeLinejoin="round" />
            </svg>
          </span>
          <span>Agent Arena</span>
        </div>
        <nav className="flex items-center gap-5">
          <Link href="/how-it-works" className="transition-colors hover:text-foreground">
            How it works
          </Link>
          <Link href="/leaderboard" className="transition-colors hover:text-foreground">
            Leaderboard
          </Link>
          <Link href="/live" className="transition-colors hover:text-foreground">
            Live arena
          </Link>
        </nav>
      </div>
    </footer>
  )
}
