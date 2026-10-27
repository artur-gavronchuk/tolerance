import { planPrompt } from '@studio/prompts';
import {
  type AssessmentJson,
  type ChangeRequest,
  PlanDraftJson,
  type Project,
  planDraftProblems,
  type Run,
} from '@studio/schema';
import { projectContext } from './assessor';
import { type AgentDeps, executeStructuredRun, RunSlots } from './structured-run';

/**
 * Turns a task into two plan variants (SPEC §3.5, Planner). The agent sees the task, the earlier
 * assessment and the project map; its answer must pass the schema and the plan checks, with up to
 * two retries. One run per task at a time.
 */
export class Planner {
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
      () => db.startRun({ projectId: project.id, requestId: request.id, kind: 'plan', agentId: 'claude' }),
      async (run, signal) => {
        // Known when the prompt is built; components outside the map are sent back as problems.
        let componentIds: Set<string> | undefined;
        await executeStructuredRun(this.deps, {
          run,
          cwd: project.repoPath,
          prompt: async () => {
            const [map, assessment] = await Promise.all([
              db.latestRepoMap(project.id),
              db.latestAssessment(request.id),
            ]);
            if (map) componentIds = new Set(map.json.components.map((c) => c.id));
            return planPrompt({
              task: request.text,
              assessment: assessment ? formatAssessment(assessment.json) : 'No assessment yet.',
              context: await projectContext(db, project),
              language: opts.language === 'ru' ? 'Russian' : 'English',
            });
          },
          schema: PlanDraftJson,
          problems: (draft) => planDraftProblems(draft, componentIds),
          model: opts.model,
          signal,
          save: async (draft) => {
            await db.addPlan({
              projectId: project.id,
              requestId: request.id,
              runId: run.id,
              variants: draft.variants,
            });
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

/** The assessment as short notes for the planner: what was understood and what is still open. */
export function formatAssessment(a: AssessmentJson): string {
  const lines = [
    `Understanding: ${a.understanding}`,
    `Verdict: ${a.verdict}. Complexity: ${a.complexity} (${a.complexityReason}).`,
    `Opinion: ${a.opinion}`,
  ];
  if (a.affected.length > 0) {
    lines.push('Likely affected:', ...a.affected.map((f) => `- ${f.path}${f.note ? ` — ${f.note}` : ''}`));
  }
  if (a.risks.length > 0) lines.push('Risks:', ...a.risks.map((r) => `- ${r}`));
  if (a.questions.length > 0) {
    lines.push('Open questions (make a reasonable assumption and state it in the approach):');
    lines.push(...a.questions.map((q) => `- ${q}`));
  }
  return lines.join('\n');
}
