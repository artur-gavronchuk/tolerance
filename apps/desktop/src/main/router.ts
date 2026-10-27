import { basename } from 'node:path';
import { detectClaude } from '@studio/agents';
import type { Db } from '@studio/db';
import { type Mapper, scanRepo } from '@studio/engine';
import { defaultBranch, NotAGitRepoError, repoRoot } from '@studio/git';
import { initTRPC, TRPCError } from '@trpc/server';
import { app, BrowserWindow, dialog } from 'electron';
import { z } from 'zod';

export interface Context {
  db: Db;
  mapper: Mapper;
}

const t = initTRPC.context<Context>().create({ isServer: true });

const projectInput = z.object({ projectId: z.string() });

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

    start: t.procedure
      .input(projectInput.extend({ language: z.enum(['ru', 'en']) }))
      .mutation(async ({ ctx, input }) => {
        const project = await requireProject(ctx.db, input.projectId);
        return ctx.mapper.start(project, { language: input.language });
      }),

    cancel: t.procedure.input(projectInput).mutation(({ ctx, input }) => ctx.mapper.cancel(input.projectId)),
  }),

  runs: t.router({
    events: t.procedure
      .input(z.object({ runId: z.string(), afterSeq: z.number().default(0) }))
      .query(({ ctx, input }) => ctx.db.listEvents(input.runId, input.afterSeq)),
  }),
});

export type AppRouter = typeof router;
