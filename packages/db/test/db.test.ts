import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { type Db, openDb } from '../src';

const migrations = fileURLToPath(new URL('../drizzle', import.meta.url));

describe('Db', () => {
  let dir: string;
  let db: Db;

  beforeEach(async () => {
    dir = mkdtempSync(join(tmpdir(), 'studio-db-'));
    db = await openDb(join(dir, 'studio.db'), migrations);
  });
  afterEach(() => {
    db.close();
    rmSync(dir, { recursive: true, force: true });
  });

  it('migrates twice without failing', async () => {
    db.close();
    db = await openDb(join(dir, 'studio.db'), migrations);
    expect(await db.listProjects()).toEqual([]);
  });

  it('stores projects, maps, runs and ordered events', async () => {
    const project = await db.addProject({ name: 'arena', repoPath: '/tmp/arena', defaultBranch: 'main' });
    expect(await db.findProjectByPath('/tmp/arena')).toEqual(project);

    const run = await db.startRun({ projectId: project.id, kind: 'map', agentId: 'claude' });
    await db.appendEvent(run.id, { type: 'tool_call', name: 'Read', kind: 'read', target: 'go.mod' });
    await db.appendEvent(run.id, { type: 'assistant_message', text: 'done' });
    const events = await db.listEvents(run.id);
    expect(events.map((e) => e.seq)).toEqual([1, 2]);
    expect(await db.listEvents(run.id, 1)).toHaveLength(1);

    await db.finishRun(run.id, { status: 'done', costUsd: 0.1 });
    expect((await db.latestRun(project.id, 'map'))?.status).toBe('done');

    const json = { summary: 's', components: [], edges: [], entrypoints: [], testCommandGuess: '' };
    await db.addRepoMap({ projectId: project.id, commitSha: 'abc', json });
    expect((await db.latestRepoMap(project.id))?.json).toEqual(json);

    await db.removeProject(project.id);
    expect(await db.latestRepoMap(project.id)).toBeUndefined();
  });

  it('stores tasks with their assessments and deletes them with their runs', async () => {
    const project = await db.addProject({ name: 'p', repoPath: '/tmp/p', defaultBranch: 'main' });
    const first = await db.addRequest({ projectId: project.id, text: 'добавить голосование' });
    const second = await db.addRequest({ projectId: project.id, text: 'рейтинг' });
    expect((await db.listRequests(project.id)).map((r) => r.id).sort()).toEqual([first.id, second.id].sort());

    const run = await db.startRun({
      projectId: project.id,
      requestId: first.id,
      kind: 'assess',
      agentId: 'claude',
    });
    await db.appendEvent(run.id, { type: 'tool_call', name: 'Read', kind: 'read', target: 'go.mod' });
    const json = {
      understanding: 'u',
      verdict: 'go' as const,
      opinion: 'o',
      complexity: 'M' as const,
      complexityReason: 'r',
      affected: [{ path: 'api/', note: 'routes' }],
      approach: ['a'],
      risks: [],
      questions: ['q'],
    };
    await db.addAssessment({ requestId: first.id, runId: run.id, json });
    expect((await db.latestAssessment(first.id))?.json).toEqual(json);
    expect((await db.latestRequestRun(first.id, 'assess'))?.id).toBe(run.id);
    expect(await db.latestRequestRun(second.id, 'assess')).toBeUndefined();

    await db.removeRequest(first.id);
    expect(await db.getRequest(first.id)).toBeUndefined();
    expect(await db.latestAssessment(first.id)).toBeUndefined();
    expect(await db.listEvents(run.id)).toEqual([]);
    expect(await db.listRequests(project.id)).toHaveLength(1);
  });

  it('marks runs left running as interrupted', async () => {
    const project = await db.addProject({ name: 'p', repoPath: '/tmp/p', defaultBranch: 'main' });
    await db.startRun({ projectId: project.id, kind: 'map', agentId: 'claude' });
    await db.markInterruptedRuns();
    expect((await db.latestRun(project.id, 'map'))?.status).toBe('interrupted');
  });

  it('stores started plan tasks and implement runs with their heads, and deletes them with the request', async () => {
    const project = await db.addProject({ name: 'p', repoPath: '/tmp/p', defaultBranch: 'main' });
    expect(project.allowedCommands).toEqual([]);
    expect((await db.getProject(project.id))?.allowedCommands).toEqual([]);
    const request = await db.addRequest({ projectId: project.id, text: 'README' });
    const planRun = await db.startRun({
      projectId: project.id,
      requestId: request.id,
      kind: 'plan',
      agentId: 'claude',
    });
    const plan = await db.addPlan({
      projectId: project.id,
      requestId: request.id,
      runId: planRun.id,
      variants: [],
    });
    await db.setPlanStatus(plan.id, 'executing');
    expect((await db.getPlan(plan.id))?.status).toBe('executing');

    const task = await db.addTask({
      planId: plan.id,
      key: 'T1',
      branch: 'studio/abc/T1-readme',
      worktreePath: '/wt/T1',
      baseRef: 'main',
    });
    expect(await db.findTask(plan.id, 'T1')).toEqual(task);
    await expect(db.addTask({ ...task })).rejects.toThrow();

    const run = await db.startRun({
      projectId: project.id,
      requestId: request.id,
      taskId: task.id,
      kind: 'implement',
      agentId: 'claude',
    });
    await db.setRunShas(run.id, { beforeSha: 'a1' });
    await db.setRunShas(run.id, { afterSha: 'b2' });
    expect(await db.latestTaskRun(task.id)).toMatchObject({
      taskId: task.id,
      beforeSha: 'a1',
      afterSha: 'b2',
    });

    await db.setTaskWorktree(task.id, null);
    expect((await db.getTask(task.id))?.worktreePath).toBeNull();
    expect(await db.listTasks(plan.id)).toHaveLength(1);

    await db.removeRequest(request.id);
    expect(await db.getTask(task.id)).toBeUndefined();
    expect(await db.latestTaskRun(task.id)).toBeUndefined();
  });

  it('stores workspaces newest first, marks interrupted setups and deletes them with the project', async () => {
    const project = await db.addProject({ name: 'arena', repoPath: '/tmp/arena', defaultBranch: 'main' });
    await db.updateProjectSettings(project.id, {
      setupCommand: 'pnpm install',
      editorCommand: 'cursor',
      terminalApp: null,
    });
    expect(await db.getProject(project.id)).toMatchObject({
      setupCommand: 'pnpm install',
      editorCommand: 'cursor',
    });

    const base = { projectId: project.id, baseRef: 'origin/main', baseSha: 'abc' };
    const first = await db.addWorkspace({
      ...base,
      name: 'Первый',
      slug: 'pervyy',
      branch: 'studio/ws/pervyy',
      path: '/w/pervyy',
      setupStatus: 'running',
    });
    await new Promise((r) => setTimeout(r, 2));
    const second = await db.addWorkspace({
      ...base,
      name: 'second',
      slug: 'second',
      branch: 'studio/ws/second',
      path: '/w/second',
      setupStatus: 'none',
    });
    expect((await db.listWorkspaces(project.id)).map((w) => w.id)).toEqual([second.id, first.id]);

    await db.markInterruptedSetups();
    expect((await db.getWorkspace(first.id))?.setupStatus).toBe('interrupted');
    expect((await db.getWorkspace(second.id))?.setupStatus).toBe('none');

    await db.setWorkspaceSetup(second.id, { status: 'failed', exitCode: 1, log: 'boom' });
    expect(await db.getWorkspace(second.id)).toMatchObject({
      setupStatus: 'failed',
      setupExitCode: 1,
      setupLog: 'boom',
    });

    await db.removeWorkspace(second.id);
    expect(await db.getWorkspace(second.id)).toBeUndefined();
    await db.removeProject(project.id);
    expect(await db.listWorkspaces(project.id)).toEqual([]);
  });
});
