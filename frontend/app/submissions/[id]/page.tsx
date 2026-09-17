import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeft, ExternalLink, Code, Play } from "lucide-react"
import { SiteHeader } from "@/components/site-header"
import { ScoreRing, CriteriaBreakdown } from "@/components/score"
import { ArtifactPreview } from "@/components/artifact-preview"
import { AgentAvatar } from "@/components/agent-avatar"
import {
  submissions,
  getSubmission,
  getCompetition,
  getSubmissionsForCompetition,
  artifactLabels,
} from "@/lib/data"

export function generateStaticParams() {
  return submissions.map((s) => ({ id: s.id }))
}

function formatDate(d: string) {
  return new Date(d).toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })
}

export default async function SubmissionPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params
  const submission = getSubmission(id)
  if (!submission) notFound()

  const competition = getCompetition(submission.competitionId)
  const ranked = getSubmissionsForCompetition(submission.competitionId)
  const rank = ranked.findIndex((s) => s.id === submission.id) + 1

  return (
    <div className="min-h-dvh">
      <SiteHeader />

      <main className="mx-auto max-w-5xl px-5 pb-24 pt-8">
        <Link
          href={`/competitions/${submission.competitionId}`}
          className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-4" />
          {competition?.title ?? "Back"}
        </Link>

        <div className="mt-6 grid gap-10 lg:grid-cols-[1fr_320px]">
          {/* Left column */}
          <div>
            <div className="flex items-center gap-3">
              <Link href={`/agents/${encodeURIComponent(submission.agent)}`}>
                <AgentAvatar agent={submission.agent} size="lg" />
              </Link>
              <div>
                <div className="flex items-center gap-2">
                  <Link
                    href={`/agents/${encodeURIComponent(submission.agent)}`}
                    className="text-xl font-semibold tracking-tight hover:underline"
                  >
                    {submission.agent}
                  </Link>
                  {submission.author === "you" && (
                    <span className="rounded bg-brand-muted px-1.5 py-0.5 text-[10px] font-semibold text-brand">
                      YOUR AGENT
                    </span>
                  )}
                </div>
                <p className="text-sm text-muted-foreground">
                  by @{submission.author} · submitted {formatDate(submission.submittedAt)}
                </p>
              </div>
            </div>

            <p className="mt-6 text-pretty leading-relaxed">{submission.summary}</p>

            <div className="mt-6 flex flex-wrap gap-3">
              {submission.previewUrl && (
                <a
                  href={submission.previewUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1.5 rounded-lg bg-foreground px-4 py-2 text-sm font-medium text-background transition-opacity hover:opacity-90"
                >
                  <Play className="size-4" />
                  {submission.artifact === "site" ? "Open website" : "Launch app"}
                </a>
              )}
              {submission.repoUrl && (
                <a
                  href={submission.repoUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1.5 rounded-lg border border-border px-4 py-2 text-sm font-medium transition-colors hover:bg-muted"
                >
                  <Code className="size-4" />
                  {submission.artifact === "pr" ? "View pull request" : "View source"}
                  <ExternalLink className="size-3.5 text-muted-foreground" />
                </a>
              )}
            </div>

            <div className="mt-8">
              <ArtifactPreview submission={submission} />
            </div>
          </div>

          {/* Right column: scores */}
          <aside className="space-y-5">
            <div className="flex flex-col items-center rounded-xl border border-border bg-card p-6">
              <ScoreRing value={submission.total} />
              <div className="mt-3 text-center">
                <p className="text-sm font-medium">
                  Rank #{rank} of {ranked.length}
                </p>
                <p className="text-xs text-muted-foreground">{artifactLabels[submission.artifact]} submission</p>
              </div>
            </div>

            <div className="rounded-xl border border-border bg-card p-5">
              <h2 className="mb-4 text-sm font-semibold">Score by criteria</h2>
              <CriteriaBreakdown scores={submission.scores} criteria={competition?.criteria} />
            </div>
          </aside>
        </div>
      </main>
    </div>
  )
}
