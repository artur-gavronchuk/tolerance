import { existsSync } from 'node:fs';
import { AgentCancelledError, runClaudeImplement } from '@studio/agents';
import {
  commitAll,
  createWorktree,
  currentBranch,
  defaultWorktreesRoot,
  diffFromMergeBase,
  git,
  removeWorktree,
  resolveCommit,
  type TaskDiff,
  taskWorktreeLayout,
} from '@studio/git';
import { implementPrompt } from '@studio/prompts';
import type { PlanWithVariants, Project, Run, Task, TaskSpecJson } from '@studio/schema';
import { projectContext } from './assessor';
import { type AgentDeps, eventRecorder, RunSlots } from './structured-run';

export interface ImplementerDeps extends AgentDeps {
  /** The implement call; tests replace it with a fake. */
  runImplement?: typeof runClaudeImplement;
  /** `~/.studio/worktrees` unless set (tests). */
  worktreesRoot?: string;
}

/** Why a task cannot be started or its worktree removed right now; the message is for the user. */
export class TaskStateError extends Error {}

/**
 * Runs one task of an approved plan with an agent in its own worktree (SPEC §3.4, §3.5 p.5).
 * Tasks are started by hand, one at a time per plan: no scheduler, no integration branch yet.
 */
export class Implementer {
  private readonly slots: RunSlots;
  private readonly running = new Map<string, string>(); // plan id → task id

  constructor(private readonly deps: ImplementerDeps) {
    this.slots = new RunSlots(deps.notify);
  }

  get root(): string {
    return this.deps.worktreesRoot ?? defaultWorktreesRoot();
  }

  /** The implement run in progress for a plan, if any. */
  activeRun(planId: string): Run | undefined {
    return this.slots.active(planId);
  }

  async start(
    project: Project,
    plan: PlanWithVariants,
    taskKey: string,
    opts: { language: 'ru' | 'en'; model?: string },
  ): Promise<Run> {
    const { db, notify } = this.deps;
    if (plan.status !== 'approved' && plan.status !== 'executing') {
      throw new TaskStateError('The plan is not approved');
    }
    const spec = plan.variants
      .find((v) => v.id === plan.chosenVariantId)
      ?.json.tasks.find((t) => t.key === taskKey);
    if (!spec) throw new TaskStateError(`Task ${taskKey} is not in the approved variant`);
    const active = this.slots.active(plan.id);
    if (active && this.running.get(plan.id) !== (await db.findTask(plan.id, taskKey))?.id) {
      throw new TaskStateError('Another task of this plan is running; tasks run one at a time');
    }

    let task: Task | undefined;
    return this.slots.start(
      plan.id,
      async () => {
        task = (await db.findTask(plan.id, taskKey)) ?? (await this.addTask(project, plan, spec));
        this.running.set(plan.id, task.id);
        const run = await db.startRun({
          projectId: project.id,
          requestId: plan.requestId,
          taskId: task.id,
          kind: 'implement',
          agentId: 'claude',
        });
        if (plan.status === 'approved') await db.setPlanStatus(plan.id, 'executing');
        notify({ type: 'request', projectId: project.id, requestId: plan.requestId });
        return run;
      },
      async (run, signal) => {
        if (!task) return;
        try {
          await this.execute(project, plan, task, spec, run, signal, opts);
        } finally {
          this.running.delete(plan.id);
          notify({ type: 'request', projectId: project.id, requestId: plan.requestId });
        }
      },
    );
  }

  cancel(planId: string): void {
    this.slots.cancel(planId);
  }

  /** What the task changed: from its worktree while there is one, else from its branch. */
  async diff(project: Project, task: Task): Promise<TaskDiff | null> {
    if (task.worktreePath && existsSync(task.worktreePath)) {
      return diffFromMergeBase({ cwd: task.worktreePath, base: task.baseRef });
    }
    const branch = await resolveCommit(project.repoPath, task.branch).catch(() => null);
    if (!branch) return null;
    return diffFromMergeBase({ cwd: project.repoPath, base: task.baseRef, head: task.branch });
  }

  /** Removes the task's worktree; its branch and commits stay, and the diff is still shown from the branch. */
  async removeWorktree(project: Project, task: Task): Promise<void> {
    if (this.running.get(task.planId) === task.id) {
      throw new TaskStateError('The task is running; stop it first');
    }
    if (task.worktreePath) {
      await removeWorktree({ repo: project.repoPath, root: this.root, path: task.worktreePath });
    }
    await this.deps.db.setTaskWorktree(task.id, null);
  }

  private async addTask(project: Project, plan: PlanWithVariants, spec: TaskSpecJson): Promise<Task> {
    const { branch } = this.layout(project, plan, spec);
    return this.deps.db.addTask({
      planId: plan.id,
      key: spec.key,
      branch,
      worktreePath: null,
      baseRef: await baseRef(project),
    });
  }

  private layout(project: Project, plan: PlanWithVariants, spec: TaskSpecJson) {
    return taskWorktreeLayout({
      root: this.root,
      projectName: project.name,
      planId: plan.id,
      taskKey: spec.key,
      taskTitle: spec.title,
    });
  }

  private async execute(
    project: Project,
    plan: PlanWithVariants,
    task: Task,
    spec: TaskSpecJson,
    run: Run,
    signal: AbortSignal,
    opts: { language: 'ru' | 'en'; model?: string },
  ): Promise<void> {
    const { db } = this.deps;
    const runImplement = this.deps.runImplement ?? runClaudeImplement;
    const { record, flush } = eventRecorder(this.deps, run);
    let path: string | null = null;
    let costUsd: number | undefined;

    try {
      path = task.worktreePath ?? this.layout(project, plan, spec).path;
      await createWorktree({ repo: project.repoPath, path, branch: task.branch, base: task.baseRef });
      await db.setTaskWorktree(task.id, path);
      await db.setRunShas(run.id, { beforeSha: await git(path, ['rev-parse', 'HEAD']) });

      const result = await runImplement({
        cwd: path,
        prompt: implementPrompt({
          key: spec.key,
          title: spec.title,
          description: spec.description,
          allow: bullets(spec.scope.allow, (g) => `\`${g}\``),
          deny: bullets(spec.scope.deny, (g) => `\`${g}\``),
          criteria: bullets(
            spec.acceptance,
            (c) => `${c.id} (${c.check}${c.command ? `: \`${c.command}\`` : ''}) — ${c.text}`,
          ),
          context: await projectContext(db, project),
          commands:
            project.allowedCommands.length > 0
              ? `only commands starting with ${project.allowedCommands.map((c) => `\`${c}\``).join(', ')}; anything else is refused.`
              : 'not available in this run. Do not try to run commands; say in the final message what should be run.',
          language: opts.language === 'ru' ? 'Russian' : 'English',
        }),
        model: opts.model,
        allowedCommands: project.allowedCommands,
        onEvent: record,
        signal,
      });
      costUsd = result.costUsd;

      const { sha } = await commitAll(path, `studio: ${spec.key} ${spec.title}`);
      await db.setRunShas(run.id, { afterSha: sha });
      await flush();
      await db.finishRun(run.id, { status: 'done', costUsd });
    } catch (error) {
      await flush();
      if (path && existsSync(path)) {
        const head = await git(path, ['rev-parse', 'HEAD']).catch(() => null);
        await db.setRunShas(run.id, { afterSha: head });
      }
      if (error instanceof AgentCancelledError || signal.aborted) {
        await db.finishRun(run.id, { status: 'cancelled', costUsd });
      } else {
        const message = error instanceof Error ? error.message : String(error);
        await db.appendEvent(run.id, { type: 'error', message });
        await db.finishRun(run.id, { status: 'failed', error: message, costUsd });
      }
    }
  }
}

/**
 * The user's branch tasks start from: the project's default branch when it exists locally, else the
 * checked-out branch. Kept by name so the diff is taken from the merge-base with it (SPEC §3.4).
 */
async function baseRef(project: Project): Promise<string> {
  if (await resolveCommit(project.repoPath, `refs/heads/${project.defaultBranch}`).catch(() => null)) {
    return project.defaultBranch;
  }
  const current = await currentBranch(project.repoPath);
  if (current && (await resolveCommit(project.repoPath, current).catch(() => null))) return current;
  throw new TaskStateError('The repository has no commits on its main branch yet');
}

function bullets<T>(items: T[], format: (item: T) => string): string {
  return items.length > 0 ? items.map((i) => `- ${format(i)}`).join('\n') : '- (none)';
}
