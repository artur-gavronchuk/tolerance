import type { Db } from '@studio/db';
import { listFiles } from '@studio/git';
import { assessPrompt } from '@studio/prompts';
import { AssessmentJson, type ChangeRequest, type Project, type RepoMap, type Run } from '@studio/schema';
import { formatScan, formatTree } from './mapper';
import { scanRepo } from './scan';
import { type AgentDeps, executeStructuredRun, RunSlots } from './structured-run';

/**
 * An agent reads the task and the code and gives its opinion before any planning:
 * understanding, verdict, size, affected files, risks, questions. One run per task at a time.
 */
export class Assessor {
  private readonly slots: RunSlots;

  constructor(private readonly deps: AgentDeps) {
    this.slots = new RunSlots(deps.notify);
  }

  activeRun(requestId: string): Run | undefined {
    return this.slots.active(requestId);
  }

  async start(
    project: Project,
    request: ChangeRequest,
    opts: { language: 'ru' | 'en'; model?: string },
  ): Promise<Run> {
    const { db, notify } = this.deps;
    return this.slots.start(
      request.id,
      () => db.startRun({ projectId: project.id, requestId: request.id, kind: 'assess', agentId: 'claude' }),
      async (run, signal) => {
        await executeStructuredRun(this.deps, {
          run,
          cwd: project.repoPath,
          prompt: async () =>
            assessPrompt({
              task: request.text,
              context: await projectContext(db, project),
              language: opts.language === 'ru' ? 'Russian' : 'English',
            }),
          schema: AssessmentJson,
          model: opts.model,
          signal,
          save: async (json) => {
            await db.addAssessment({ requestId: request.id, runId: run.id, json });
            notify({ type: 'request', projectId: project.id, requestId: request.id });
          },
        });
      },
    );
  }

  cancel(requestId: string): void {
    this.slots.cancel(requestId);
  }
}

/** The project map when there is one (it is what the agent already learned), else the quick scan and layout. */
export async function projectContext(db: Db, project: Project): Promise<string> {
  const map = await db.latestRepoMap(project.id);
  if (map) return formatMap(map);
  const [scan, files] = await Promise.all([scanRepo(project.repoPath), listFiles(project.repoPath)]);
  return `### Quick scan\n\n${formatScan(scan)}\n\n### Layout\n\n${formatTree(files)}`;
}

export function formatMap(map: RepoMap): string {
  const { summary, components, edges } = map.json;
  const lines = [`### Project map\n\n${summary}\n`];
  for (const c of components) {
    lines.push(`- ${c.id} (${c.kind}, ${c.paths.join(', ') || 'no paths'}): ${c.description}`);
  }
  if (edges.length > 0) {
    lines.push('', 'Connections:');
    for (const e of edges) lines.push(`- ${e.from} ${e.kind} ${e.to}${e.note ? ` (${e.note})` : ''}`);
  }
  return lines.join('\n');
}
