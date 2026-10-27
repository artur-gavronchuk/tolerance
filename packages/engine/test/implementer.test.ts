import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { AgentFailedError, type ImplementRunOptions } from '@studio/agents';
import { type Db, openDb } from '@studio/db';
import { git } from '@studio/git';
import type { PlanVariantJson, PlanWithVariants, Project, TaskSpecJson } from '@studio/schema';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Implementer, TaskStateError } from '../src';

const migrations = fileURLToPath(new URL('../../db/drizzle', import.meta.url));

function task(key: string, title: string): TaskSpecJson {
  return {
    key,
    title,
    description: `Сделать ${key}`,
    dependsOn: [],
    scope: { allow: ['README.md'], deny: ['src/**'] },
    acceptance: [
      { id: `${key}.A1`, text: 'В README есть раздел', check: 'command', command: 'grep -q Run README.md' },
    ],
    agentHint: 'claude',
  };
}

const variant: PlanVariantJson = {
  title: 'README',
  approach: 'Дописать README',
  tradeoffs: { pros: [], cons: [], risks: [] },
  complexity: 'S',
  affectedComponents: [],
  tasks: [task('T1', 'Раздел «Запуск»'), task('T2', 'Раздел «Тесты»')],
};

describe('Implementer', () => {
  let tmp: string;
  let repo: string;
  let root: string;
  let db: Db;
  let project: Project;
  let plan: PlanWithVariants;

  beforeEach(async () => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-impl-')));
    repo = join(tmp, 'repo');
    root = join(tmp, 'worktrees');
    mkdirSync(repo);
    await git(repo, ['init', '-q', '-b', 'main']);
    await git(repo, ['config', 'user.name', 't']);
    await git(repo, ['config', 'user.email', 't@t']);
    writeFileSync(join(repo, 'README.md'), '# demo\n');
    await git(repo, ['add', '-A']);
    await git(repo, ['commit', '-qm', 'init']);

    db = await openDb(join(tmp, 'studio.db'), migrations);
    project = await db.addProject({ name: 'Demo Repo', repoPath: repo, defaultBranch: 'main' });
    const request = await db.addRequest({ projectId: project.id, text: 'Добавь раздел в README' });
    const planRun = await db.startRun({
      projectId: project.id,
      requestId: request.id,
      kind: 'plan',
      agentId: 'claude',
    });
    const draft = await db.addPlan({
      projectId: project.id,
      requestId: request.id,
      runId: planRun.id,
      variants: [variant, variant],
    });
    await db.approvePlan(draft.id, draft.variants[0]?.id ?? '');
    plan = (await db.getPlan(draft.id)) as PlanWithVariants;
  });
  afterEach(() => {
    db.close();
    rmSync(tmp, { recursive: true, force: true });
  });

  /** A stand-in agent that edits the README in its working directory, like Claude Code would. */
  function editingAgent(calls: ImplementRunOptions[]) {
    return async (opts: ImplementRunOptions) => {
      calls.push(opts);
      writeFileSync(join(opts.cwd, 'README.md'), '# demo\n\n## Run\n\npnpm dev\n');
      writeFileSync(join(opts.cwd, 'NOTES.md'), 'untracked note\n');
      opts.onEvent({ type: 'tool_call', name: 'Edit', kind: 'edit', target: 'README.md' });
      opts.onEvent({ type: 'assistant_message', text: 'Добавил раздел.' });
      return { sessionId: 's', costUsd: 0.4, finalMessage: 'Добавил раздел.' };
    };
  }

  it('runs a task in its own worktree, commits for the agent and leaves the main branch alone', async () => {
    const calls: ImplementRunOptions[] = [];
    const notify = vi.fn();
    const implementer = new Implementer({
      db,
      notify,
      worktreesRoot: root,
      runImplement: editingAgent(calls),
    });
    const mainHead = await git(repo, ['rev-parse', 'HEAD']);

    const run = await implementer.start(project, plan, 'T1', { language: 'ru' });
    expect(run).toMatchObject({ kind: 'implement', status: 'running', requestId: plan.requestId });
    await vi.waitFor(() => expect(implementer.activeRun(plan.id)).toBeUndefined());

    const t1 = await db.findTask(plan.id, 'T1');
    const path = join(root, 'demo-repo', plan.id, 'T1');
    expect(t1).toMatchObject({
      worktreePath: path,
      branch: `studio/${plan.id.slice(0, 8)}/T1-razdel-zapusk`,
      baseRef: 'main',
    });

    // The agent worked in the worktree, with the task in its prompt and no shell.
    expect(calls[0]?.cwd).toBe(path);
    expect(calls[0]?.allowedCommands).toEqual([]);
    const prompt = calls[0]?.prompt ?? '';
    expect(prompt).toContain('T1 Раздел «Запуск»');
    expect(prompt).toContain('- `README.md`');
    expect(prompt).toContain('- `src/**`');
    expect(prompt).toContain('T1.A1 (command: `grep -q Run README.md`) — В README есть раздел');
    expect(prompt).toContain('not available in this run');
    expect(prompt).toContain('final message in Russian');

    const done = await db.latestTaskRun(t1?.id ?? '');
    expect(done).toMatchObject({ status: 'done', costUsd: 0.4, beforeSha: mainHead });
    expect(done?.afterSha).toBe(await git(path, ['rev-parse', 'HEAD']));
    expect(await git(path, ['log', '-1', '--format=%s'])).toBe('studio: T1 Раздел «Запуск»');
    expect(await git(path, ['status', '--porcelain'])).toBe('');
    expect((await db.listEvents(run.id)).map((e) => e.event.type)).toEqual([
      'tool_call',
      'assistant_message',
    ]);
    expect((await db.getPlan(plan.id))?.status).toBe('executing');

    const diff = await implementer.diff(project, t1 as NonNullable<typeof t1>);
    expect(diff?.files.map((f) => [f.path, f.status])).toEqual([
      ['NOTES.md', 'added'],
      ['README.md', 'modified'],
    ]);
    expect(diff?.patch).toContain('+## Run');

    expect(await git(repo, ['rev-parse', 'HEAD'])).toBe(mainHead);
    expect(await git(repo, ['status', '--porcelain'])).toBe('');
    expect(readFileSync(join(repo, 'README.md'), 'utf8')).toBe('# demo\n');
  });

  it('removes the worktree but keeps the branch and its diff, and works there again on the next run', async () => {
    const implementer = new Implementer({
      db,
      notify: () => {},
      worktreesRoot: root,
      runImplement: editingAgent([]),
    });
    await implementer.start(project, plan, 'T1', { language: 'en' });
    await vi.waitFor(() => expect(implementer.activeRun(plan.id)).toBeUndefined());
    const t1 = (await db.findTask(plan.id, 'T1')) as NonNullable<Awaited<ReturnType<Db['findTask']>>>;
    const path = t1.worktreePath ?? '';

    await implementer.removeWorktree(project, t1);
    expect(existsSync(path)).toBe(false);
    const removed = (await db.getTask(t1.id)) as typeof t1;
    expect(removed.worktreePath).toBeNull();
    expect((await implementer.diff(project, removed))?.files.map((f) => f.path)).toEqual([
      'NOTES.md',
      'README.md',
    ]);

    const firstAfter = (await db.latestTaskRun(t1.id))?.afterSha;
    await implementer.start(project, plan, 'T1', { language: 'en' });
    await vi.waitFor(() => expect(implementer.activeRun(plan.id)).toBeUndefined());
    expect(existsSync(path)).toBe(true);
    expect((await db.latestTaskRun(t1.id))?.beforeSha).toBe(firstAfter);
  });

  it('runs one task of a plan at a time and only tasks of the approved variant', async () => {
    const gate: { release?: () => void } = {};
    const implementer = new Implementer({
      db,
      notify: () => {},
      worktreesRoot: root,
      runImplement: () =>
        new Promise((resolve) => {
          gate.release = () => resolve({ sessionId: null, costUsd: 0, finalMessage: '' });
        }),
    });
    const run = await implementer.start(project, plan, 'T1', { language: 'ru' });
    expect((await implementer.start(project, plan, 'T1', { language: 'ru' })).id).toBe(run.id);
    await expect(implementer.start(project, plan, 'T2', { language: 'ru' })).rejects.toBeInstanceOf(
      TaskStateError,
    );
    await expect(implementer.start(project, plan, 'T9', { language: 'ru' })).rejects.toBeInstanceOf(
      TaskStateError,
    );
    const t1 = await db.findTask(plan.id, 'T1');
    await expect(implementer.removeWorktree(project, t1 as NonNullable<typeof t1>)).rejects.toBeInstanceOf(
      TaskStateError,
    );

    await vi.waitFor(() => expect(gate.release).toBeDefined());
    gate.release?.();
    await vi.waitFor(() => expect(implementer.activeRun(plan.id)).toBeUndefined());
    // Nothing changed: no commit, the run still records where it ended.
    const done = await db.latestTaskRun(t1?.id ?? '');
    expect(done).toMatchObject({ status: 'done' });
    expect(done?.afterSha).toBe(done?.beforeSha);
  });

  it('records a failed run and keeps the agent work uncommitted', async () => {
    const implementer = new Implementer({
      db,
      notify: () => {},
      worktreesRoot: root,
      runImplement: async (opts) => {
        writeFileSync(join(opts.cwd, 'README.md'), 'half done\n');
        throw new AgentFailedError('max turns');
      },
    });
    await implementer.start(project, plan, 'T1', { language: 'ru' });
    await vi.waitFor(() => expect(implementer.activeRun(plan.id)).toBeUndefined());
    const t1 = await db.findTask(plan.id, 'T1');
    const failed = await db.latestTaskRun(t1?.id ?? '');
    expect(failed).toMatchObject({ status: 'failed', error: 'max turns' });
    expect(failed?.afterSha).toBe(failed?.beforeSha);
    expect(await git(t1?.worktreePath ?? '', ['status', '--porcelain'])).toBe(' M README.md');
    // The diff still shows the uncommitted work.
    expect((await implementer.diff(project, t1 as NonNullable<typeof t1>))?.files.map((f) => f.path)).toEqual(
      ['README.md'],
    );
  });

  it('marks a cancelled run', async () => {
    const implementer = new Implementer({
      db,
      notify: () => {},
      worktreesRoot: root,
      runImplement: (opts) =>
        new Promise((_, reject) => {
          opts.signal?.addEventListener('abort', () => reject(new Error('killed')));
        }),
    });
    await implementer.start(project, plan, 'T1', { language: 'ru' });
    // Generous timeouts: git is slow when the whole suite runs in parallel.
    await vi.waitFor(async () => expect((await db.findTask(plan.id, 'T1'))?.worktreePath).toBeTruthy(), 5000);
    implementer.cancel(plan.id);
    await vi.waitFor(() => expect(implementer.activeRun(plan.id)).toBeUndefined(), 5000);
    const t1 = await db.findTask(plan.id, 'T1');
    expect((await db.latestTaskRun(t1?.id ?? ''))?.status).toBe('cancelled');
  });

  it('fails the run with a clear message when the repository has no commits', async () => {
    rmSync(join(repo, '.git'), { recursive: true });
    await git(repo, ['init', '-q', '-b', 'main']);
    const implementer = new Implementer({
      db,
      notify: () => {},
      worktreesRoot: root,
      runImplement: editingAgent([]),
    });
    await expect(implementer.start(project, plan, 'T1', { language: 'ru' })).rejects.toThrow(/no commits/);
  });

  it('refuses a plan that is not approved', async () => {
    await db.reopenPlan(plan.id);
    const reopened = (await db.getPlan(plan.id)) as PlanWithVariants;
    const implementer = new Implementer({ db, notify: () => {}, worktreesRoot: root });
    await expect(implementer.start(project, reopened, 'T1', { language: 'ru' })).rejects.toBeInstanceOf(
      TaskStateError,
    );
  });
});
