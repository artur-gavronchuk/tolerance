import { basename } from 'node:path';
import { detectClaude } from '@studio/agents';
import type { Db } from '@studio/db';
import { type Assessor, type Mapper, scanRepo } from '@studio/engine';
import { defaultBranch, NotAGitRepoError, repoRoot } from '@studio/git';
import { initTRPC, TRPCError } from '@trpc/server';
import { app, BrowserWindow, dialog } from 'electron';
import { z } from 'zod';

export interface Context {
  db: Db;
  mapper: Mapper;
  assessor: Assessor;
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
      for (const request of await ctx.db.listRequests(input.projectId)) ctx.assessor.cancel(request.id);
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
        requests.map(async (request) => ({
          request,
          assessment: (await ctx.db.latestAssessment(request.id)) ?? null,
          run: (await ctx.db.latestRequestRun(request.id, 'assess')) ?? null,
        })),
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
      await ctx.db.removeRequest(input.requestId);
    }),
  }),

  runs: t.router({
    events: t.procedure
      .input(z.object({ runId: z.string(), afterSeq: z.number().default(0) }))
      .query(({ ctx, input }) => ctx.db.listEvents(input.runId, input.afterSeq)),
  }),
});

export type AppRouter = typeof router;
