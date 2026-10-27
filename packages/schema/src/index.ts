import { z } from 'zod';

// ── Project ─────────────────────────────────────────────────────────────────

export const Project = z.object({
  id: z.string(),
  name: z.string(),
  repoPath: z.string(),
  defaultBranch: z.string(),
  testCommand: z.string().nullable(),
  setupCommand: z.string().nullable(),
  createdAt: z.number(),
});
export type Project = z.infer<typeof Project>;

// ── Quick scan: what the repository is made of, found without an LLM ───────

export const ScanCategory = z.enum(['frontend', 'backend', 'database', 'infra', 'testing', 'docs']);
export type ScanCategory = z.infer<typeof ScanCategory>;

export const ScanFinding = z.object({
  category: ScanCategory,
  /**
   * What was found. A technology name such as "Next.js 15" or "PostgreSQL", or, when `generic`
   * is true, a key such as "sqlMigrations" that the UI translates.
   */
  label: z.string(),
  generic: z.boolean(),
  /** How many files matched; evidence lists at most a dozen of them. */
  count: z.number(),
  /** Files that prove it, relative to the repository root. */
  evidence: z.array(z.string()),
});
export type ScanFinding = z.infer<typeof ScanFinding>;

export const LanguageShare = z.object({
  name: z.string(),
  files: z.number(),
  bytes: z.number(),
  /** Share of all bytes in recognised source files, 0..1. */
  share: z.number(),
});
export type LanguageShare = z.infer<typeof LanguageShare>;

export const RepoScan = z.object({
  commitSha: z.string().nullable(),
  branch: z.string().nullable(),
  totalFiles: z.number(),
  languages: z.array(LanguageShare),
  findings: z.array(ScanFinding),
});
export type RepoScan = z.infer<typeof RepoScan>;

// ── Repo map: what an agent found reading the code (SPEC §3.2) ──────────────

export const ComponentKind = z.enum([
  'frontend',
  'backend',
  'service',
  'library',
  'db',
  'infra',
  'test',
  'other',
]);
export type ComponentKind = z.infer<typeof ComponentKind>;

export const EdgeKind = z.enum(['calls', 'imports', 'reads', 'writes', 'publishes', 'other']);
export type EdgeKind = z.infer<typeof EdgeKind>;

export const RepoMapComponent = z.object({
  id: z.string().describe('Short stable kebab-case id, e.g. "web-app" or "postgres"'),
  name: z.string(),
  kind: ComponentKind,
  paths: z.array(z.string()).describe('Directories or files relative to the repo root'),
  description: z.string().describe('What it does, 1-3 sentences'),
  tech: z.array(z.string()).describe('Frameworks, languages and libraries it is built with'),
  entities: z
    .array(z.string())
    .describe('For db: tables or collections. For backend/service: main API routes or handlers. Else empty'),
});
export type RepoMapComponent = z.infer<typeof RepoMapComponent>;

export const RepoMapEdge = z.object({
  from: z.string().describe('Component id'),
  to: z.string().describe('Component id'),
  kind: EdgeKind,
  note: z.string().describe('How they are connected, a few words; empty if obvious'),
});
export type RepoMapEdge = z.infer<typeof RepoMapEdge>;

export const RepoMapJson = z.object({
  summary: z.string().describe('What this product is and who it is for, 2-4 sentences'),
  components: z.array(RepoMapComponent),
  edges: z.array(RepoMapEdge),
  entrypoints: z.array(z.string()).describe('Files where execution starts: servers, CLIs, app roots'),
  testCommandGuess: z.string().describe('Command that runs the tests from the repo root; empty if none'),
});
export type RepoMapJson = z.infer<typeof RepoMapJson>;

/** Checks that a map is consistent beyond its shape. Returns human-readable problems. */
export function repoMapProblems(map: RepoMapJson): string[] {
  const problems: string[] = [];
  const ids = new Set<string>();
  for (const c of map.components) {
    if (ids.has(c.id)) problems.push(`Duplicate component id "${c.id}"`);
    ids.add(c.id);
  }
  for (const e of map.edges) {
    if (!ids.has(e.from)) problems.push(`Edge from unknown component "${e.from}"`);
    if (!ids.has(e.to)) problems.push(`Edge to unknown component "${e.to}"`);
  }
  return problems;
}

export const RepoMap = z.object({
  id: z.string(),
  projectId: z.string(),
  commitSha: z.string().nullable(),
  json: RepoMapJson,
  createdAt: z.number(),
});
export type RepoMap = z.infer<typeof RepoMap>;

// ── Change requests: the task a developer brings, and the agent's opinion of it ──

/**
 * What the developer wants changed, in their own words. Shown as "задача" in the UI; named
 * ChangeRequest in code so it is not confused with a plan's Task (T1, T2…). Plans grow out of it later.
 */
export const ChangeRequest = z.object({
  id: z.string(),
  projectId: z.string(),
  text: z.string(),
  createdAt: z.number(),
});
export type ChangeRequest = z.infer<typeof ChangeRequest>;

export const AssessmentVerdict = z.enum(['go', 'clarify', 'reconsider']);
export type AssessmentVerdict = z.infer<typeof AssessmentVerdict>;

export const Complexity = z.enum(['S', 'M', 'L']);
export type Complexity = z.infer<typeof Complexity>;

/** An agent's read of a task before any planning: does it make sense here, how big, what it touches. */
export const AssessmentJson = z.object({
  understanding: z.string().describe('How you understood the task, in your own words, 1-3 sentences'),
  verdict: AssessmentVerdict.describe(
    'go: clear and sensible, can be planned; clarify: open questions block a good plan; reconsider: the task conflicts with the code or has a better alternative',
  ),
  opinion: z
    .string()
    .describe(
      'Your honest opinion as a senior engineer on this codebase: is it worth doing, how, what to watch, 3-6 sentences',
    ),
  complexity: Complexity.describe(
    'S: hours, one area; M: a day or two, several areas; L: many days or a redesign',
  ),
  complexityReason: z.string().describe('Why this size, one sentence'),
  affected: z
    .array(z.object({ path: z.string(), note: z.string().describe('What changes there, a few words') }))
    .describe('Files or directories the change will most likely touch, relative to the repo root'),
  approach: z.array(z.string()).describe('Rough steps to get it done, 2-6 short items'),
  risks: z.array(z.string()).describe('What can break or go wrong; empty if nothing notable'),
  questions: z.array(z.string()).describe('What to ask the author before planning; empty if all is clear'),
});
export type AssessmentJson = z.infer<typeof AssessmentJson>;

export const Assessment = z.object({
  id: z.string(),
  requestId: z.string(),
  runId: z.string(),
  json: AssessmentJson,
  createdAt: z.number(),
});
export type Assessment = z.infer<typeof Assessment>;

// ── Runs and agent events ───────────────────────────────────────────────────

export const RunKind = z.enum([
  'map',
  'assess',
  'plan',
  'implement',
  'follow_up',
  'resolve_conflict',
  'verify',
]);
export type RunKind = z.infer<typeof RunKind>;

export const RunStatus = z.enum(['running', 'done', 'failed', 'cancelled', 'interrupted']);
export type RunStatus = z.infer<typeof RunStatus>;

export const Run = z.object({
  id: z.string(),
  projectId: z.string(),
  /** The change request this run works on; null for project-wide runs such as the map. */
  requestId: z.string().nullable(),
  kind: RunKind,
  agentId: z.string(),
  status: RunStatus,
  error: z.string().nullable(),
  costUsd: z.number().nullable(),
  startedAt: z.number(),
  endedAt: z.number().nullable(),
});
export type Run = z.infer<typeof Run>;

export const ToolKind = z.enum(['read', 'edit', 'command', 'search', 'fetch', 'other']);
export type ToolKind = z.infer<typeof ToolKind>;

/** Agent output normalised across agents (SPEC §3.3). */
export const NormalizedEvent = z.discriminatedUnion('type', [
  z.object({ type: z.literal('assistant_message'), text: z.string() }),
  z.object({ type: z.literal('thinking'), text: z.string() }),
  z.object({
    type: z.literal('tool_call'),
    name: z.string(),
    kind: ToolKind,
    /** Short human-readable target: a path, a pattern or a command. */
    target: z.string(),
  }),
  z.object({ type: z.literal('usage'), costUsd: z.number() }),
  z.object({ type: z.literal('error'), message: z.string() }),
  z.object({ type: z.literal('turn_end') }),
]);
export type NormalizedEvent = z.infer<typeof NormalizedEvent>;

export const AgentEvent = z.object({
  runId: z.string(),
  seq: z.number(),
  ts: z.number(),
  event: NormalizedEvent,
});
export type AgentEvent = z.infer<typeof AgentEvent>;

// ── Notifications from main to the UI ───────────────────────────────────────

/** Something changed that the UI shows; it refetches what it needs. */
export type StudioEvent =
  | { type: 'run'; projectId: string; runId: string }
  | { type: 'map'; projectId: string }
  | { type: 'request'; projectId: string; requestId: string };
