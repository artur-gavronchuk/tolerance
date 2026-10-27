import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { StructuredRunOptions } from '@studio/agents';
import { type Db, openDb } from '@studio/db';
import { git } from '@studio/git';
import {
  findCycle,
  type PlanVariantJson,
  type Project,
  planDraftProblems,
  planVariantProblems,
  type TaskSpecJson,
} from '@studio/schema';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Planner } from '../src';

const migrations = fileURLToPath(new URL('../../db/drizzle', import.meta.url));

function task(key: string, dependsOn: string[] = [], extra: Partial<TaskSpecJson> = {}): TaskSpecJson {
  return {
    key,
    title: `Задача ${key}`,
    description: 'Сделать',
    dependsOn,
    scope: { allow: [`api/${key}/**`], deny: [] },
    acceptance: [{ id: `${key}.A1`, text: 'Тест проходит', check: 'test', command: '' }],
    agentHint: 'claude',
    ...extra,
  };
}

function variant(title: string, tasks: TaskSpecJson[], componentIds: string[] = []): PlanVariantJson {
  return {
    title,
    approach: 'Подход',
    tradeoffs: { pros: [], cons: [], risks: [] },
    complexity: 'M',
    affectedComponents: componentIds.map((componentId) => ({ componentId, change: 'modify', note: '' })),
    tasks,
  };
}

describe('plan checks', () => {
  it('accepts a valid DAG', () => {
    const v = variant('ok', [task('T1'), task('T2'), task('T3', ['T1', 'T2'])], ['api']);
    expect(planVariantProblems(v, new Set(['api']))).toEqual([]);
  });

  it('finds duplicate keys, unknown and self dependencies, missing criteria and unknown components', () => {
    const v = variant(
      'bad',
      [
        task('T1', ['T1']),
        task('T1'),
        task('T2', ['T9'], { acceptance: [] }),
        task('T3', [], {
          acceptance: [{ id: 'T3.A1', text: 'команда', check: 'command', command: ' ' }],
          scope: { allow: [], deny: [] },
        }),
      ],
      ['nowhere'],
    );
    expect(planVariantProblems(v, new Set(['api']))).toEqual(
      expect.arrayContaining([
        'Duplicate task key "T1"',
        'Duplicate criterion id "T1.A1"',
        'Task T2 has no acceptance criteria',
        'Criterion T3.A1 is check=command but has no command',
        'Task T3 has an empty scope.allow',
        'Task T1 depends on itself',
        'Task T2 depends on unknown task "T9"',
        'Unknown component "nowhere" (not on the project map)',
      ]),
    );
  });

  it('skips the component check when there is no map', () => {
    expect(planVariantProblems(variant('x', [task('T1')], ['anything']))).toEqual([]);
  });

  it('reports a dependency cycle with its path', () => {
    expect(findCycle([task('T1', ['T3']), task('T2', ['T1']), task('T3', ['T2'])])).toEqual([
      'T1',
      'T3',
      'T2',
      'T1',
    ]);
    expect(findCycle([task('T1'), task('T2', ['T1'])])).toBeNull();
  });

  it('prefixes problems with the variant number', () => {
    const draft = { variants: [variant('a', [task('T1')]), variant('b', [task('T1', ['T2'])])] };
    expect(planDraftProblems(draft)).toEqual(['Variant 2: Task T1 depends on unknown task "T2"']);
  });
});

describe('Planner', () => {
  let dir: string;
  let db: Db;
  let project: Project;

  beforeEach(async () => {
    dir = realpathSync(mkdtempSync(join(tmpdir(), 'studio-planner-')));
    await git(dir, ['init', '-q', '-b', 'main']);
    mkdirSync(dirname(join(dir, 'api/main.go')), { recursive: true });
    writeFileSync(join(dir, 'api/main.go'), 'package main\n');
    db = await openDb(join(dir, 'studio.db'), migrations);
    project = await db.addProject({ name: 'p', repoPath: dir, defaultBranch: 'main' });
  });
  afterEach(() => {
    db.close();
    rmSync(dir, { recursive: true, force: true });
  });

  it('plans with the task, assessment and map, sends plan problems back and saves both variants', async () => {
    await db.addRepoMap({
      projectId: project.id,
      commitSha: null,
      json: {
        summary: 'Арена',
        components: [
          {
            id: 'api',
            name: 'API',
            kind: 'backend',
            paths: ['api'],
            description: '',
            tech: [],
            entities: [],
          },
        ],
        edges: [],
        entrypoints: [],
        testCommandGuess: '',
      },
    });
    const request = await db.addRequest({ projectId: project.id, text: 'Добавить голосование' });
    const assessRun = await db.startRun({
      projectId: project.id,
      requestId: request.id,
      kind: 'assess',
      agentId: 'claude',
    });
    await db.addAssessment({
      requestId: request.id,
      runId: assessRun.id,
      json: {
        understanding: 'Голосование за решения',
        verdict: 'clarify',
        opinion: 'Нормально',
        complexity: 'M',
        complexityReason: 'Таблица и экран',
        affected: [],
        approach: [],
        risks: [],
        questions: ['Кто голосует?'],
      },
    });

    const good = {
      variants: [
        variant('В существующем API', [task('T1'), task('T2', ['T1'])], ['api']),
        variant('Отдельный сервис', [task('T1')], ['api']),
      ],
    };
    const cyclic = {
      variants: [good.variants[0], variant('Цикл', [task('T1', ['T2']), task('T2', ['T1'])], ['web'])],
    };
    const prompts: StructuredRunOptions[] = [];
    const notify = vi.fn();
    const planner = new Planner({
      db,
      notify,
      runAgent: async (opts) => {
        prompts.push(opts);
        return { output: prompts.length === 1 ? cyclic : good, sessionId: 's1', costUsd: 0.3 };
      },
    });

    await planner.start(project, request, { language: 'ru' });
    await vi.waitFor(() => expect(planner.activeRun(request.id)).toBeUndefined());

    expect(prompts[0]?.prompt).toContain('Добавить голосование');
    expect(prompts[0]?.prompt).toContain('Кто голосует?');
    expect(prompts[0]?.prompt).toContain('- api (backend');
    expect(prompts[0]?.prompt).toContain('in Russian');
    expect(prompts[1]?.resumeSessionId).toBe('s1');
    expect(prompts[1]?.prompt).toContain('Variant 2: Unknown component "web"');
    expect(prompts[1]?.prompt).toContain('Variant 2: Dependency cycle');

    const run = await db.latestRequestRun(request.id, 'plan');
    expect(run).toMatchObject({ status: 'done', costUsd: 0.6 });
    const plan = await db.latestPlan(request.id);
    expect(plan).toMatchObject({ status: 'awaiting_choice', chosenVariantId: null, runId: run?.id });
    expect(plan?.variants.map((v) => v.json.title)).toEqual(['В существующем API', 'Отдельный сервис']);
    expect(notify).toHaveBeenCalledWith({ type: 'request', projectId: project.id, requestId: request.id });

    const chosen = plan?.variants[1];
    await db.approvePlan(plan?.id ?? '', chosen?.id ?? '');
    expect(await db.latestPlan(request.id)).toMatchObject({
      status: 'approved',
      chosenVariantId: chosen?.id,
    });
    await db.reopenPlan(plan?.id ?? '');
    expect(await db.latestPlan(request.id)).toMatchObject({ status: 'awaiting_choice', approvedAt: null });
  });

  it('fails the run and saves no plan when the agent never fixes its answer', async () => {
    const request = await db.addRequest({ projectId: project.id, text: 'x' });
    const planner = new Planner({
      db,
      notify: () => {},
      runAgent: async () => ({
        output: { variants: [variant('один', [task('T1')])] },
        sessionId: 's',
        costUsd: 0,
      }),
    });
    await planner.start(project, request, { language: 'en' });
    await vi.waitFor(() => expect(planner.activeRun(request.id)).toBeUndefined());

    expect(await db.latestRequestRun(request.id, 'plan')).toMatchObject({ status: 'failed' });
    expect(await db.latestPlan(request.id)).toBeUndefined();
  });

  it('deletes plans with their task', async () => {
    const request = await db.addRequest({ projectId: project.id, text: 'x' });
    const run = await db.startRun({
      projectId: project.id,
      requestId: request.id,
      kind: 'plan',
      agentId: 'claude',
    });
    await db.addPlan({
      projectId: project.id,
      requestId: request.id,
      runId: run.id,
      variants: [variant('a', [task('T1')]), variant('b', [task('T1')])],
    });
    await db.removeRequest(request.id);
    expect(await db.latestPlan(request.id)).toBeUndefined();
  });
});
