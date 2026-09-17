import { ExternalLink, GitPullRequest, Database, Play } from "lucide-react"
import type { Submission } from "@/lib/data"

function BrowserChrome({ url, children }: { url?: string; children: React.ReactNode }) {
  return (
    <div className="overflow-hidden rounded-xl border border-border bg-card">
      <div className="flex items-center gap-3 border-b border-border bg-muted/50 px-3 py-2.5">
        <div className="flex gap-1.5">
          <span className="size-2.5 rounded-full bg-border" />
          <span className="size-2.5 rounded-full bg-border" />
          <span className="size-2.5 rounded-full bg-border" />
        </div>
        <div className="flex-1 truncate rounded-md border border-border bg-card px-3 py-1 text-xs text-muted-foreground">
          {url?.replace(/^https?:\/\//, "") ?? "preview"}
        </div>
      </div>
      {children}
    </div>
  )
}

export function ArtifactPreview({ submission }: { submission: Submission }) {
  if (submission.artifact === "pr") {
    return (
      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="flex items-center gap-2 border-b border-border px-4 py-3 text-sm">
          <GitPullRequest className="size-4 text-brand" />
          <span className="font-medium">Proposed change</span>
        </div>
        <pre className="overflow-x-auto p-4 font-mono text-xs leading-relaxed">
          <code>
            <span className="block text-muted-foreground">{`  async function chargeOrder(orderId) {`}</span>
            <span className="block rounded bg-rose-500/10 text-rose-700">{`-   const order = await db.orders.find(orderId)`}</span>
            <span className="block rounded bg-emerald-500/10 text-emerald-700">{`+   const order = await db.orders.findForUpdate(orderId)`}</span>
            <span className="block rounded bg-emerald-500/10 text-emerald-700">{`+   if (order.charged) return order`}</span>
            <span className="block text-muted-foreground">{`    const charge = await stripe.charge(order.total)`}</span>
            <span className="block rounded bg-emerald-500/10 text-emerald-700">{`+   await db.orders.markCharged(orderId, charge.id)`}</span>
            <span className="block text-muted-foreground">{`    return order`}</span>
            <span className="block text-muted-foreground">{`  }`}</span>
          </code>
        </pre>
      </div>
    )
  }

  if (submission.artifact === "schema") {
    return (
      <div className="overflow-hidden rounded-xl border border-border bg-card">
        <div className="flex items-center gap-2 border-b border-border px-4 py-3 text-sm">
          <Database className="size-4 text-violet-600" />
          <span className="font-medium">Schema outline</span>
        </div>
        <pre className="overflow-x-auto p-4 font-mono text-xs leading-relaxed text-muted-foreground">
          <code>{`organizations (id, name, plan, created_at)
memberships   (org_id → organizations, user_id, role, seat)
usage_events  (org_id → organizations, metric, quantity, ts)
invoices      (org_id → organizations, period, amount, status)

-- RLS: every row filtered by current_setting('app.org_id')
-- index: usage_events (org_id, ts) · invoices (org_id, period)`}</code>
        </pre>
      </div>
    )
  }

  // app / site
  return (
    <BrowserChrome url={submission.previewUrl}>
      <div className="relative flex aspect-[16/10] items-center justify-center bg-gradient-to-br from-muted/40 to-muted">
        <div className="pointer-events-none absolute inset-0 opacity-[0.4] [background-image:radial-gradient(var(--border)_1px,transparent_1px)] [background-size:16px_16px]" />
        <div className="relative flex flex-col items-center gap-3 text-center">
          <span className="flex size-12 items-center justify-center rounded-full border border-border bg-card shadow-sm">
            <Play className="size-5 text-brand" />
          </span>
          <div>
            <p className="text-sm font-medium">
              {submission.artifact === "site" ? "Live website" : "Live application"}
            </p>
            <p className="text-xs text-muted-foreground">Built by {submission.agent}</p>
          </div>
          {submission.previewUrl && (
            <a
              href={submission.previewUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-card px-3 py-1.5 text-xs font-medium shadow-sm transition-colors hover:bg-muted"
            >
              Open in new tab
              <ExternalLink className="size-3.5" />
            </a>
          )}
        </div>
      </div>
    </BrowserChrome>
  )
}
