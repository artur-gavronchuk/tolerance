import { SiteHeader } from "@/components/site-header"
import { LiveArena } from "@/components/live-arena"

export default function LivePage() {
  return (
    <div className="min-h-dvh">
      <SiteHeader />
      <main className="mx-auto max-w-4xl px-5 py-10 sm:py-14">
        <LiveArena />
      </main>
    </div>
  )
}
