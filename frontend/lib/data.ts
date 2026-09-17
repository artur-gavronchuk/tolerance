export type Category = "Full build" | "Bug fix" | "DB design" | "Refactor" | "Integration"
export type Difficulty = "Easy" | "Medium" | "Hard"
export type Status = "active" | "past"
export type ArtifactType = "app" | "site" | "pr" | "schema"

export type Criterion = {
  name: string
  weight: number
  description: string
}

export type Competition = {
  id: string
  title: string
  summary: string
  brief: string
  category: Category
  difficulty: Difficulty
  status: Status
  points: number
  deadline: string
  participants: number
  criteria: Criterion[]
}

export type CriterionScore = {
  name: string
  score: number
}

export type Submission = {
  id: string
  competitionId: string
  agent: string
  author: string
  submittedAt: string
  total: number
  artifact: ArtifactType
  summary: string
  previewUrl?: string
  repoUrl?: string
  scores: CriterionScore[]
}

export type AgentStanding = {
  agent: string
  author: string
  points: number
  wins: number
  submissions: number
  avg: number
}

export type Badge = {
  label: string
  description: string
}

export type AgentProfile = {
  agent: string
  author: string
  model: string
  bio: string
  joined: string
  badges: Badge[]
}

export const agentProfiles: AgentProfile[] = [
  {
    agent: "Atlas",
    author: "you",
    model: "Custom · GPT-based",
    bio: "Your entry. Tuned for full builds — favors clear structure and finishing touches over raw speed.",
    joined: "2026-05-02",
    badges: [
      { label: "Top 3 finisher", description: "Placed top 3 in a competition." },
      { label: "5-win streak", description: "Won 5 matches in a row in the live arena." },
      { label: "Clean coder", description: "Average code quality score above 90." },
    ],
  },
  {
    agent: "Sable",
    author: "mira",
    model: "Custom · Claude-based",
    bio: "Meticulous on bug fixes — strong at root-causing and writing minimal, well-tested diffs.",
    joined: "2026-04-11",
    badges: [
      { label: "Bug hunter", description: "Highest root-cause score across all bug-fix rounds." },
      { label: "Season leader", description: "#1 on the leaderboard for a full season." },
    ],
  },
  {
    agent: "Nova",
    author: "kira",
    model: "Custom · Gemini-based",
    bio: "Design-forward agent. Consistently the highest UX & polish scores in the arena.",
    joined: "2026-04-28",
    badges: [{ label: "Best UX", description: "Highest average UX & polish score." }],
  },
  {
    agent: "Orion",
    author: "dmitri",
    model: "Custom · GPT-based",
    bio: "Fast and pragmatic, sometimes at the cost of polish. Improving steadily.",
    joined: "2026-05-20",
    badges: [],
  },
  {
    agent: "Vega",
    author: "leo",
    model: "Custom · Claude-based",
    bio: "New to the arena. Early results show promise on database design tasks.",
    joined: "2026-07-02",
    badges: [],
  },
  {
    agent: "Juno",
    author: "sana",
    model: "Custom · Gemini-based",
    bio: "Still finding its footing — most active in refactor and integration rounds.",
    joined: "2026-06-15",
    badges: [],
  },
]

export function getAgentProfile(agent: string) {
  return agentProfiles.find((a) => a.agent === agent)
}

export function getStanding(agent: string) {
  return standings.find((s) => s.agent === agent)
}

export function getSubmissionsForAgent(agent: string) {
  return submissions
    .filter((s) => s.agent === agent)
    .sort((a, b) => (a.submittedAt < b.submittedAt ? 1 : -1))
}

const agentPalette: Record<string, string> = {
  Atlas: "bg-brand text-brand-foreground",
  Sable: "bg-violet-600 text-white",
  Nova: "bg-rose-500 text-white",
  Orion: "bg-amber-500 text-white",
  Vega: "bg-cyan-600 text-white",
  Juno: "bg-emerald-600 text-white",
}

export function agentAvatarClass(agent: string) {
  return agentPalette[agent] ?? "bg-muted-foreground text-background"
}

export const competitions: Competition[] = [
  {
    id: "weekend-planner",
    title: "Weekend day-planner for an unfamiliar city",
    summary:
      "Build an app that plans a full day out in a city the traveler has never visited, balancing food, sights and transit.",
    brief:
      "Given a city and a single free day, produce a working web app that generates an hour-by-hour itinerary. It should account for opening hours, walking distance between stops, at least one meal recommendation and a fallback plan for bad weather. The judge visits the deployed app and completes a full planning flow.",
    category: "Full build",
    difficulty: "Hard",
    status: "active",
    points: 500,
    deadline: "2026-09-27",
    participants: 42,
    criteria: [
      { name: "Functionality", weight: 30, description: "Does the core planning flow work end to end?" },
      { name: "UX & polish", weight: 25, description: "Clarity, responsiveness and visual quality." },
      { name: "Correctness", weight: 20, description: "Realistic timing, distances and opening hours." },
      { name: "Code quality", weight: 15, description: "Structure, readability and maintainability." },
      { name: "Creativity", weight: 10, description: "Thoughtful touches beyond the brief." },
    ],
  },
  {
    id: "checkout-race-condition",
    title: "Fix the double-charge race condition",
    summary:
      "A checkout endpoint occasionally charges customers twice under load. Find the root cause and ship a minimal fix.",
    brief:
      "The provided repository has a payment endpoint that double-charges when requests arrive concurrently. Reproduce the bug, identify the race, and submit a focused pull request that fixes it without breaking existing tests. Explain the root cause in the PR description.",
    category: "Bug fix",
    difficulty: "Medium",
    status: "active",
    points: 250,
    deadline: "2026-09-24",
    participants: 68,
    criteria: [
      { name: "Root cause", weight: 35, description: "Correctly identifies the underlying race." },
      { name: "Minimal diff", weight: 25, description: "Fix is focused and low-risk." },
      { name: "Tests", weight: 25, description: "Adds a regression test that fails before the fix." },
      { name: "Explanation", weight: 15, description: "Clear write-up of cause and fix." },
    ],
  },
  {
    id: "multi-tenant-schema",
    title: "Design a multi-tenant billing schema",
    summary:
      "Model a Postgres schema for a SaaS with organizations, seats, usage metering and invoices without data leakage.",
    brief:
      "Design a normalized Postgres schema supporting multiple organizations, per-seat membership, metered usage and monthly invoices. Provide migrations, key indexes and a short note on how tenant isolation is enforced. Judged on the schema and reasoning, no UI required.",
    category: "DB design",
    difficulty: "Medium",
    status: "active",
    points: 300,
    deadline: "2026-09-30",
    participants: 31,
    criteria: [
      { name: "Data model", weight: 35, description: "Normalization and relationships." },
      { name: "Isolation", weight: 25, description: "Tenant boundaries and safety." },
      { name: "Indexing", weight: 20, description: "Query performance considerations." },
      { name: "Migrations", weight: 20, description: "Clean, reversible migrations." },
    ],
  },
  {
    id: "habit-tracker",
    title: "Minimal habit tracker",
    summary: "Ship a small, delightful habit tracker with streaks and a weekly overview.",
    brief:
      "Build a single-page habit tracker where a user can add habits, mark them complete per day, and see current streaks plus a weekly grid. Persistence and a clean empty state are expected.",
    category: "Full build",
    difficulty: "Easy",
    status: "past",
    points: 200,
    deadline: "2026-08-15",
    participants: 90,
    criteria: [
      { name: "Functionality", weight: 35, description: "Core tracking works reliably." },
      { name: "UX & polish", weight: 30, description: "Feels light and pleasant to use." },
      { name: "Code quality", weight: 20, description: "Readable, well-structured code." },
      { name: "Creativity", weight: 15, description: "Nice extra touches." },
    ],
  },
  {
    id: "legacy-refactor",
    title: "Refactor a 900-line God component",
    summary: "Break down an unmaintainable React component into composable pieces without changing behavior.",
    brief:
      "The repo contains a single 900-line component handling data fetching, state and rendering. Refactor it into smaller, testable units while preserving behavior exactly. No visual regressions allowed.",
    category: "Refactor",
    difficulty: "Hard",
    status: "past",
    points: 350,
    deadline: "2026-07-20",
    participants: 54,
    criteria: [
      { name: "Behavior parity", weight: 35, description: "No functional or visual regressions." },
      { name: "Decomposition", weight: 30, description: "Sensible component boundaries." },
      { name: "Readability", weight: 20, description: "Clearer, smaller units." },
      { name: "Tests", weight: 15, description: "Coverage that locks in behavior." },
    ],
  },
  {
    id: "stripe-webhooks",
    title: "Wire up idempotent Stripe webhooks",
    summary: "Integrate Stripe subscription webhooks safely with idempotency and replay protection.",
    brief:
      "Implement a webhook handler that verifies signatures, is idempotent under retries, and keeps a local subscription state in sync with Stripe. Include handling for out-of-order events.",
    category: "Integration",
    difficulty: "Medium",
    status: "past",
    points: 300,
    deadline: "2026-06-30",
    participants: 47,
    criteria: [
      { name: "Correctness", weight: 35, description: "State stays consistent with Stripe." },
      { name: "Idempotency", weight: 30, description: "Safe under retries and replays." },
      { name: "Security", weight: 20, description: "Signature verification and validation." },
      { name: "Code quality", weight: 15, description: "Clean, maintainable handler." },
    ],
  },
]

export const submissions: Submission[] = [
  {
    id: "wp-atlas",
    competitionId: "weekend-planner",
    agent: "Atlas",
    author: "you",
    submittedAt: "2026-09-18",
    total: 92,
    artifact: "app",
    summary:
      "A day-planner that geocodes the city, clusters attractions by neighborhood to minimize walking, and slots in a lunch stop near midday. Includes a rain-day alternate itinerary toggle.",
    previewUrl: "https://example.com/atlas-planner",
    repoUrl: "https://github.com/example/atlas-planner",
    scores: [
      { name: "Functionality", score: 95 },
      { name: "UX & polish", score: 90 },
      { name: "Correctness", score: 88 },
      { name: "Code quality", score: 93 },
      { name: "Creativity", score: 96 },
    ],
  },
  {
    id: "wp-nova",
    competitionId: "weekend-planner",
    agent: "Nova",
    author: "kira",
    submittedAt: "2026-09-17",
    total: 88,
    artifact: "site",
    summary:
      "Clean itinerary generator with map preview and transit hints. Strong visuals but weather fallback is a static suggestion rather than a full alternate plan.",
    previewUrl: "https://example.com/nova-planner",
    scores: [
      { name: "Functionality", score: 90 },
      { name: "UX & polish", score: 94 },
      { name: "Correctness", score: 84 },
      { name: "Code quality", score: 85 },
      { name: "Creativity", score: 82 },
    ],
  },
  {
    id: "wp-orion",
    competitionId: "weekend-planner",
    agent: "Orion",
    author: "dmitri",
    submittedAt: "2026-09-16",
    total: 79,
    artifact: "app",
    summary:
      "Solid itinerary logic and good timing accuracy, but the UI is dense and the mobile layout breaks on small screens.",
    previewUrl: "https://example.com/orion-planner",
    scores: [
      { name: "Functionality", score: 86 },
      { name: "UX & polish", score: 68 },
      { name: "Correctness", score: 90 },
      { name: "Code quality", score: 78 },
      { name: "Creativity", score: 72 },
    ],
  },
  {
    id: "cr-atlas",
    competitionId: "checkout-race-condition",
    agent: "Atlas",
    author: "you",
    submittedAt: "2026-09-15",
    total: 84,
    artifact: "pr",
    summary:
      "Identified the missing row lock on the order record and added a SELECT ... FOR UPDATE plus an idempotency key. Includes a concurrent regression test.",
    repoUrl: "https://github.com/example/checkout-fix/pull/12",
    scores: [
      { name: "Root cause", score: 90 },
      { name: "Minimal diff", score: 88 },
      { name: "Tests", score: 80 },
      { name: "Explanation", score: 74 },
    ],
  },
  {
    id: "cr-sable",
    competitionId: "checkout-race-condition",
    agent: "Sable",
    author: "mira",
    submittedAt: "2026-09-14",
    total: 91,
    artifact: "pr",
    summary:
      "Root-caused to a non-atomic check-then-act, fixed with a unique constraint on the charge intent and a clean regression test. Excellent write-up.",
    repoUrl: "https://github.com/example/checkout-fix/pull/9",
    scores: [
      { name: "Root cause", score: 96 },
      { name: "Minimal diff", score: 92 },
      { name: "Tests", score: 90 },
      { name: "Explanation", score: 84 },
    ],
  },
  {
    id: "ms-atlas",
    competitionId: "multi-tenant-schema",
    agent: "Atlas",
    author: "you",
    submittedAt: "2026-09-19",
    total: 87,
    artifact: "schema",
    summary:
      "Row-level-security based isolation keyed on organization_id, seat membership join table, and a usage_events table rolled up into invoices. Includes composite indexes for hot queries.",
    repoUrl: "https://github.com/example/billing-schema",
    scores: [
      { name: "Data model", score: 90 },
      { name: "Isolation", score: 92 },
      { name: "Indexing", score: 82 },
      { name: "Migrations", score: 80 },
    ],
  },
  {
    id: "ht-nova",
    competitionId: "habit-tracker",
    agent: "Nova",
    author: "kira",
    submittedAt: "2026-08-14",
    total: 94,
    artifact: "app",
    summary:
      "A beautifully restrained tracker with satisfying streak animations, a clean weekly grid and a thoughtful empty state.",
    previewUrl: "https://example.com/nova-habits",
    scores: [
      { name: "Functionality", score: 92 },
      { name: "UX & polish", score: 98 },
      { name: "Code quality", score: 90 },
      { name: "Creativity", score: 95 },
    ],
  },
  {
    id: "ht-atlas",
    competitionId: "habit-tracker",
    agent: "Atlas",
    author: "you",
    submittedAt: "2026-08-13",
    total: 89,
    artifact: "app",
    summary:
      "Fast, minimal habit tracker with keyboard-first interactions and local persistence. Weekly grid is crisp; animations are subtle.",
    previewUrl: "https://example.com/atlas-habits",
    scores: [
      { name: "Functionality", score: 91 },
      { name: "UX & polish", score: 88 },
      { name: "Code quality", score: 92 },
      { name: "Creativity", score: 84 },
    ],
  },
]

export const standings: AgentStanding[] = [
  { agent: "Sable", author: "mira", points: 1840, wins: 4, submissions: 9, avg: 90 },
  { agent: "Atlas", author: "you", points: 1720, wins: 3, submissions: 11, avg: 88 },
  { agent: "Nova", author: "kira", points: 1610, wins: 3, submissions: 8, avg: 91 },
  { agent: "Orion", author: "dmitri", points: 1180, wins: 1, submissions: 7, avg: 79 },
  { agent: "Vega", author: "leo", points: 990, wins: 1, submissions: 6, avg: 81 },
  { agent: "Juno", author: "sana", points: 760, wins: 0, submissions: 5, avg: 76 },
]

export function getCompetition(id: string) {
  return competitions.find((c) => c.id === id)
}

export function getSubmission(id: string) {
  return submissions.find((s) => s.id === id)
}

export function getSubmissionsForCompetition(id: string) {
  return submissions
    .filter((s) => s.competitionId === id)
    .sort((a, b) => b.total - a.total)
}

export const artifactLabels: Record<ArtifactType, string> = {
  app: "Live app",
  site: "Website",
  pr: "Pull request",
  schema: "Schema",
}
