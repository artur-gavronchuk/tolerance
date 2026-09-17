import Link from "next/link"

const nav = [
  { href: "/", label: "Arena" },
  { href: "/live", label: "Live" },
  { href: "/leaderboard", label: "Leaderboard" },
  { href: "/how-it-works", label: "How it works" },
]

export function SiteHeader() {
  return (
    <header className="sticky top-0 z-40 border-b border-border/80 bg-background/80 backdrop-blur-md">
      <div className="mx-auto flex h-14 max-w-6xl items-center justify-between px-5">
        <Link href="/" className="flex items-center gap-2.5">
          <span className="flex size-6 items-center justify-center rounded-md bg-foreground text-background">
            <svg width="13" height="13" viewBox="0 0 16 16" fill="none" aria-hidden="true">
              <path d="M8 1.5 14 5v6l-6 3.5L2 11V5l6-3.5Z" stroke="currentColor" strokeWidth="1.3" strokeLinejoin="round" />
              <path d="M8 8 14 5M8 8v6.5M8 8 2 5" stroke="currentColor" strokeWidth="1.3" strokeLinejoin="round" />
            </svg>
          </span>
          <span className="text-sm font-semibold tracking-tight">Agent Arena</span>
        </Link>

        <nav className="flex items-center gap-1">
          {nav.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="rounded-md px-3 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              {item.label}
            </Link>
          ))}
          <Link
            href="/dashboard"
            className="ml-2 flex items-center gap-2 rounded-full border border-border py-1 pl-1 pr-3 transition-colors hover:border-foreground/30"
          >
            <span className="flex size-6 items-center justify-center rounded-full bg-brand text-[11px] font-medium text-brand-foreground">
              A
            </span>
            <span className="text-xs font-medium">Atlas</span>
          </Link>
        </nav>
      </div>
    </header>
  )
}
