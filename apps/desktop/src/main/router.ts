import { basename } from 'node:path';
import { detectClaude } from '@studio/agents';
import type { Db } from '@studio/db';
import {
  type Assessor,
  type Implementer,
  type Mapper,
  type Planner,
  scanRepo,
  TaskStateError,
} from '@studio/engine';
import { defaultBranch, NotAGitRepoError, repoRoot } from '@studio/git';
import { type Plan, planVariantProblems } from '@studio/schema';
import { initTRPC, TRPCError } from '@trpc/server';
import { app, BrowserWindow, dialog } from 'electron';
import { z } from 'zod';
import { broadcast } from './ipc';

export interface Context {
  db: Db;
  mapper: Mapper;
  assessor: Assessor;
  planner: Planner;
  implementer: Implementer;
}

const t = initTRPC.context<Context>().create({ isServer: true });

const projectInput = z.object({ projectId: z.string() });

const language = z.enum(['ru', 'en']);

async function requireRequest(db: Db, id: string) {
  const request = await db.getRequest(id);
  if (!request) throw new TRPCError({ code: 'NOT_FOUND', message: 'Task not found' });
  return request;
}

async function requireProject(db: Db, id: string) {
  const project = await db.getProject(id);
  if (!project) throw new TRPCError({ code: 'NOT_FOUND', message: 'Project not found' });
  return project;
}

async function requireTask(db: Db, id: string) {
  const task = await db.getTask(id);
  if (!task) throw new TRPCError({ code: 'NOT_FOUND', message: 'Task not found' });
  return task;
}

async function requirePlan(db: Db, id: string) {
  const plan = await db.getPlan(id);
  if (!plan) throw new TRPCError({ code: 'NOT_FOUND', message: 'Plan not found' });
  return plan;
}

/** A task that cannot be started or cleaned up right now is a conflict, not a server error. */
async function asConflict<T>(work: () => Promise<T>): Promise<T> {
  try {
    return await work();
  } catch (error) {
    if (error instanceof TaskStateError) {
      throw new TRPCError({ code: 'CONFLICT', message: error.message, cause: error });
    }
    throw error;
  }
}

function notifyRequest(plan: Pick<Plan, 'projectId' | 'requestId'>) {
  broadcast({ type: 'request', projectId: plan.projectId, requestId: plan.requestId });
}

export const router = t.router({
  system: t.router({
    info: t.procedure.query(async () => ({
      appVersion: app.getVersion(),
      claude: await detectClaude(),
    })),
  }),

  projects: t.router({
    list: t.procedure.query(({ ctx }) => ctx.db.listProjects()),

    /** Native folder picker; returns null when the user cancels. */
    pickFolder: t.procedure.mutation(async () => {
      const win = BrowserWindow.getFocusedWindow();
      const options = { properties: ['openDirectory' as const, 'createDirectory' as const] };
      const result = win ? await dialog.showOpenDialog(win, options) : await dialog.showOpenDialog(options);
      return result.canceled ? null : (result.filePaths[0] ?? null);
    }),

    add: t.procedure.input(z.object({ path: z.string() })).mutation(async ({ ctx, input }) => {
      let root: string;
      try {
        root = await repoRoot(input.path);
      } catch (error) {
        if (error instanceof NotAGitRepoError) {
          throw new TRPCError({ code: 'BAD_REQUEST', message: 'not_a_git_repo', cause: error });
        }
        throw error;
      }
      const existing = await ctx.db.findProjectByPath(root);
      if (existing) return { project: existing, created: false };
      const project = await ctx.db.addProject({
        name: basename(root),
        repoPath: root,
        defaultBranch: await defaultBranch(root),
      });
      return { project, created: true };
    }),

    remove: t.procedure.input(projectInput).mutation(async ({ ctx, input }) => {
      ctx.mapper.cancel(input.projectId);
      for (const request of await ctx.db.listRequests(input.projectId)) {
        ctx.assessor.cancel(request.id);
        ctx.planner.cancel(request.id);
        const plan = await ctx.db.latestPlan(request.id);
        if (plan) ctx.implementer.cancel(plan.id);
      }
      await ctx.db.removeProject(input.projectId);
    }),

    scan: t.procedure.input(projectInput).query(async ({ ctx, input }) => {
      const project = await requireProject(ctx.db, input.projectId);
      return scanRepo(project.repoPath);
    }),
  }),

  map: t.router({
    get: t.procedure.input(projectInput).query(async ({ ctx, input }) => ({
      map: (await ctx.db.latestRepoMap(input.projectId)) ?? null,
      run: (await ctx.db.latestRun(input.projectId, 'map')) ?? null,
    })),

    start: t.procedure.input(projectInput.extend({ language })).mutation(async ({ ctx, input }) => {
      const project = await requireProject(ctx.db, input.projectId);
      return ctx.mapper.start(project, { language: input.language });
    }),

    cancel: t.procedure.input(projectInput).mutation(({ ctx, input }) => ctx.mapper.cancel(input.projectId)),
  }),

  /** Tasks the developer brings (ChangeRequest), each with the agent's assessment. */
  requests: t.router({
    list: t.procedure.input(projectInput).query(async ({ ctx, input }) => {
      const requests = await ctx.db.listRequests(input.projectId);
      return Promise.all(
        requests.map(async (request) => {
          const plan = (await ctx.db.latestPlan(request.id)) ?? null;
          const tasks = plan ? await ctx.db.listTasks(plan.id) : [];
          return {
            request,
            assessment: (await ctx.db.latestAssessment(request.id)) ?? null,
            run: (await ctx.db.latestRequestRun(request.id, 'assess')) ?? null,
            plan,
            planRun: (await ctx.db.latestRequestRun(request.id, 'plan')) ?? null,
            /** Started tasks of the approved variant, each with its latest implement run. */
            tasks: await Promise.all(
              tasks.map(async (task) => ({ task, run: (await ctx.db.latestTaskRun(task.id)) ?? null })),
            ),
          };
        }),
      );
    }),

    /** Saves the task and hands it to the agent right away: an opinion is what the user wrote it for. */
    create: t.procedure
      .input(projectInput.extend({ text: z.string().trim().min(1).max(20_000), language }))
      .mutation(async ({ ctx, input }) => {
        const project = await requireProject(ctx.db, input.projectId);
        const request = await ctx.db.addRequest({ projectId: project.id, text: input.text });
        await ctx.assessor.start(project, request, { language: input.language });
        return request;
      }),

    assess: t.procedure
      .input(z.object({ requestId: z.string(), language }))
      .mutation(async ({ ctx, input }) => {
        const request = await requireRequest(ctx.db, input.requestId);
        const project = await requireProject(ctx.db, request.projectId);
        return ctx.assessor.start(project, request, { language: input.language });
      }),

    cancel: t.procedure
      .input(z.object({ requestId: z.string() }))
      .mutation(({ ctx, input }) => ctx.assessor.cancel(input.requestId)),

    remove: t.procedure.input(z.object({ requestId: z.string() })).mutation(async ({ ctx, input }) => {
      ctx.assessor.cancel(input.requestId);
      ctx.planner.cancel(input.requestId);
      const plan = await ctx.db.latestPlan(input.requestId);
      if (plan) ctx.implementer.cancel(plan.id);
      await ctx.db.removeRequest(input.requestId);
    }),
  }),

  /** Two plan variants for a task, then the one the developer approves (SPEC §3.5). */
  plans: t.router({
    start: t.procedure
      .input(z.object({ requestId: z.string(), language }))
      .mutation(async ({ ctx, input }) => {
        const request = await requireRequest(ctx.db, input.requestId);
        const project = await requireProject(ctx.db, request.projectId);
        const current = await ctx.db.latestPlan(request.id);
        if (current && current.status !== 'awaiting_choice' && current.status !== 'approved') {
          throw new TRPCError({ code: 'CONFLICT', message: 'The plan is already being executed' });
        }
        return ctx.planner.start(project, request, { language: input.language });
      }),

    cancel: t.procedure
      .input(z.object({ requestId: z.string() }))
      .mutation(({ ctx, input }) => ctx.planner.cancel(input.requestId)),

    approve: t.procedure
      .input(z.object({ planId: z.string(), variantId: z.string() }))
      .mutation(async ({ ctx, input }) => {
        const plan = await ctx.db.getPlan(input.planId);
        if (!plan) throw new TRPCError({ code: 'NOT_FOUND', message: 'Plan not found' });
        const variant = plan.variants.find((v) => v.id === input.variantId);
        if (!variant) throw new TRPCError({ code: 'NOT_FOUND', message: 'Variant not found' });
        if (plan.status !== 'awaiting_choice' && plan.status !== 'approved') {
          throw new TRPCError({ code: 'CONFLICT', message: 'The plan is already being executed' });
        }
        // The same checks as for the agent's answer, so a plan that slipped past them cannot be approved.
        const map = await ctx.db.latestRepoMap(plan.projectId);
        const problems = planVariantProblems(
          variant.json,
          map ? new Set(map.json.components.map((c) => c.id)) : undefined,
        );
        if (problems.length > 0) {
          throw new TRPCError({ code: 'BAD_REQUEST', message: problems.join('; ') });
        }
        await ctx.db.approvePlan(plan.id, variant.id);
        notifyRequest(plan);
      }),

    reopen: t.procedure.input(z.object({ planId: z.string() })).mutation(async ({ ctx, input }) => {
      const plan = await ctx.db.getPlan(input.planId);
      if (!plan) throw new TRPCError({ code: 'NOT_FOUND', message: 'Plan not found' });
      if (plan.status !== 'approved') {
        throw new TRPCError({ code: 'CONFLICT', message: 'Only an approved plan can be reopened' });
      }
      await ctx.db.reopenPlan(plan.id);
      notifyRequest(plan);
    }),
  }),

  /** Tasks of an approved plan, run by hand one at a time, each in its own worktree (SPEC §3.4). */
  tasks: t.router({
    run: t.procedure
      .input(z.object({ planId: z.string(), taskKey: z.string(), language }))
      .mutation(async ({ ctx, input }) => {
        const plan = await requirePlan(ctx.db, input.planId);
        const project = await requireProject(ctx.db, plan.projectId);
        return asConflict(() =>
          ctx.implementer.start(project, plan, input.taskKey, { language: input.language }),
        );
      }),

    cancel: t.procedure
      .input(z.object({ planId: z.string() }))
      .mutation(({ ctx, input }) => ctx.implementer.cancel(input.planId)),

    /** Changed files and the unified diff from the merge-base with the user's branch. */
    diff: t.procedure.input(z.object({ taskId: z.string() })).query(async ({ ctx, input }) => {
      const task = await requireTask(ctx.db, input.taskId);
      const plan = await requirePlan(ctx.db, task.planId);
      const project = await requireProject(ctx.db, plan.projectId);
      return ctx.implementer.diff(project, task);
    }),

    removeWorktree: t.procedure.input(z.object({ taskId: z.string() })).mutation(async ({ ctx, input }) => {
      const task = await requireTask(ctx.db, input.taskId);
      const plan = await requirePlan(ctx.db, task.planId);
      const project = await requireProject(ctx.db, plan.projectId);
      await asConflict(() => ctx.implementer.removeWorktree(project, task));
      notifyRequest(plan);
    }),
  }),

  runs: t.router({
    events: t.procedure
      .input(z.object({ runId: z.string(), afterSeq: z.number().default(0) }))
      .query(({ ctx, input }) => ctx.db.listEvents(input.runId, input.afterSeq)),
  }),
});

export type AppRouter = typeof router;
