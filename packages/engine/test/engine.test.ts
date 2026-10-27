import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { StructuredRunOptions } from '@studio/agents';
import { type Db, openDb } from '@studio/db';
import { git } from '@studio/git';
import type { AssessmentJson, Project, RepoMapJson } from '@studio/schema';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Assessor, formatTree, Mapper, scanRepo } from '../src';

const migrations = fileURLToPath(new URL('../../db/drizzle', import.meta.url));

function write(root: string, file: string, text: string) {
  mkdirSync(dirname(join(root, file)), { recursive: true });
  writeFileSync(join(root, file), text);
}

describe('scanRepo', () => {
  let repo: string;
  beforeEach(async () => {
    repo = realpathSync(mkdtempSync(join(tmpdir(), 'studio-scan-')));
    await git(repo, ['init', '-q', '-b', 'main']);
  });
  afterEach(() => rmSync(repo, { recursive: true, force: true }));

  it('lays a repo out into frontend, backend, database, infra and tests', async () => {
    write(repo, 'web/package.json', JSON.stringify({ dependencies: { next: '^15.1.0', react: '19.0.0' } }));
    write(repo, 'web/app/page.tsx', 'export default function Page() { return null }');
    write(repo, 'api/go.mod', 'module api\n\ngo 1.24\n\nrequire github.com/jackc/pgx/v5 v5.7.0\n');
    write(repo, 'api/main.go', 'package main\nfunc main() {}\n');
    write(repo, 'api/main_test.go', 'package main\n');
    write(repo, 'api/migrations/0001_init.sql', 'create table users (id int);');
    write(repo, 'docker-compose.yml', 'services:\n  db:\n    image: postgres:17\n');

    const scan = await scanRepo(repo);
    const labels = scan.findings.map((f) => `${f.category}:${f.label}`);
    expect(labels).toEqual(
      expect.arrayContaining([
        'frontend:Next.js 15',
        'frontend:React 19',
        'backend:Go',
        'database:PostgreSQL',
        'database:sqlMigrations',
        'infra:Docker Compose',
        'testing:goTests',
      ]),
    );
    expect(scan.findings.find((f) => f.label === 'PostgreSQL')?.evidence).toEqual([
      'api/go.mod',
      'docker-compose.yml',
    ]);
    expect(scan.languages.map((l) => l.name)).toEqual(expect.arrayContaining(['Go', 'TypeScript', 'SQL']));
    expect(scan.commitSha).toBeNull();
  });
});

describe('formatTree', () => {
  it('counts files per directory two levels deep', () => {
    expect(formatTree(['a.md', 'web/app/page.tsx', 'web/app/x/y.ts', 'web/b.ts'])).toBe(
      './ 1 files at the root\nweb/ 3\n  web/app/ 2',
    );
  });
});

describe('Mapper', () => {
  let dir: string;
  let db: Db;
  let project: Project;

  const validMap: RepoMapJson = {
    summary: 'Арена',
    components: [
      {
        id: 'web',
        name: 'Web',
        kind: 'frontend',
        paths: ['web'],
        description: 'UI',
        tech: ['Next.js'],
        entities: [],
      },
      {
        id: 'db',
        name: 'Postgres',
        kind: 'db',
        paths: [],
        description: 'Data',
        tech: [],
        entities: ['users'],
      },
    ],
    edges: [{ from: 'web', to: 'db', kind: 'reads', note: '' }],
    entrypoints: [],
    testCommandGuess: '',
  };

  beforeEach(async () => {
    dir = realpathSync(mkdtempSync(join(tmpdir(), 'studio-mapper-')));
    await git(dir, ['init', '-q', '-b', 'main']);
    db = await openDb(join(dir, 'studio.db'), migrations);
    project = await db.addProject({ name: 'p', repoPath: dir, defaultBranch: 'main' });
  });
  afterEach(() => {
    db.close();
    rmSync(dir, { recursive: true, force: true });
  });

  async function finished(mapper: Mapper) {
    await vi.waitFor(() => expect(mapper.activeRun(project.id)).toBeUndefined());
    return db.latestRun(project.id, 'map');
  }

  it('sends validation errors back and saves the corrected map', async () => {
    const broken = { ...validMap, edges: [{ from: 'web', to: 'nowhere', kind: 'calls', note: '' }] };
    const prompts: StructuredRunOptions[] = [];
    const runAgent = vi.fn(async (opts: StructuredRunOptions) => {
      prompts.push(opts);
      opts.onEvent({ type: 'tool_call', name: 'Read', kind: 'read', target: 'go.mod' });
      return { output: prompts.length === 1 ? broken : validMap, sessionId: 's1', costUsd: 0.5 };
    });
    const notify = vi.fn();
    const mapper = new Mapper({ db, notify, runAgent });

    await mapper.start(project, { language: 'ru' });
    const run = await finished(mapper);

    expect(run).toMatchObject({ status: 'done', costUsd: 1 });
    expect(prompts[0]?.prompt).toContain('in Russian');
    expect(prompts[1]?.resumeSessionId).toBe('s1');
    expect(prompts[1]?.prompt).toContain('unknown component "nowhere"');
    expect((await db.latestRepoMap(project.id))?.json).toEqual(validMap);
    expect(notify).toHaveBeenCalledWith({ type: 'map', projectId: project.id });
    const events = await db.listEvents(run?.id ?? '');
    expect(events.map((e) => e.event.type)).toEqual(['tool_call', 'error', 'tool_call']);
  });

  it('records a failed run when the agent fails', async () => {
    const mapper = new Mapper({
      db,
      notify: () => {},
      runAgent: async () => {
        throw new Error('claude: command not found');
      },
    });
    await mapper.start(project, { language: 'en' });
    expect(await finished(mapper)).toMatchObject({ status: 'failed', error: 'claude: command not found' });
    expect(await db.latestRepoMap(project.id)).toBeUndefined();
  });
});

describe('Assessor', () => {
  let dir: string;
  let db: Db;
  let project: Project;

  const assessment: AssessmentJson = {
    understanding: 'Нужно голосование за решения',
    verdict: 'clarify',
    opinion: 'Разумно, но неясно, кто голосует',
    complexity: 'M',
    complexityReason: 'Новая таблица и два экрана',
    affected: [{ path: 'api/votes.go', note: 'новые маршруты' }],
    approach: ['Таблица votes', 'API', 'UI'],
    risks: ['Накрутка'],
    questions: ['Голосуют только участники?'],
  };

  beforeEach(async () => {
    dir = realpathSync(mkdtempSync(join(tmpdir(), 'studio-assessor-')));
    await git(dir, ['init', '-q', '-b', 'main']);
    write(dir, 'api/main.go', 'package main\n');
    db = await openDb(join(dir, 'studio.db'), migrations);
    project = await db.addProject({ name: 'p', repoPath: dir, defaultBranch: 'main' });
  });
  afterEach(() => {
    db.close();
    rmSync(dir, { recursive: true, force: true });
  });

  it('gives the task and the project map to the agent and saves its assessment', async () => {
    await db.addRepoMap({
      projectId: project.id,
      commitSha: null,
      json: { summary: 'Арена задач', components: [], edges: [], entrypoints: [], testCommandGuess: '' },
    });
    const request = await db.addRequest({ projectId: project.id, text: 'Добавить голосование' });
    const prompts: StructuredRunOptions[] = [];
    const notify = vi.fn();
    const assessor = new Assessor({
      db,
      notify,
      runAgent: async (opts) => {
        prompts.push(opts);
        // The first answer misses a field, so the agent is asked to fix it.
        const output = prompts.length === 1 ? { ...assessment, verdict: 'maybe' } : assessment;
        return { output, sessionId: 's1', costUsd: 0.2 };
      },
    });

    await assessor.start(project, request, { language: 'ru' });
    await vi.waitFor(() => expect(assessor.activeRun(request.id)).toBeUndefined());

    expect(prompts[0]?.prompt).toContain('Добавить голосование');
    expect(prompts[0]?.prompt).toContain('Арена задач');
    expect(prompts[1]?.resumeSessionId).toBe('s1');
    expect(prompts[1]?.prompt).toContain('verdict');
    expect(await db.latestRequestRun(request.id, 'assess')).toMatchObject({ status: 'done', costUsd: 0.4 });
    expect((await db.latestAssessment(request.id))?.json).toEqual(assessment);
    expect(notify).toHaveBeenCalledWith({ type: 'request', projectId: project.id, requestId: request.id });
  });

  it('falls back to the quick scan without a map and records failures', async () => {
    const request = await db.addRequest({ projectId: project.id, text: 'x' });
    let prompt = '';
    const assessor = new Assessor({
      db,
      notify: () => {},
      runAgent: async (opts) => {
        prompt = opts.prompt;
        throw new Error('rate limited');
      },
    });
    await assessor.start(project, request, { language: 'en' });
    await vi.waitFor(() => expect(assessor.activeRun(request.id)).toBeUndefined());

    expect(prompt).toContain('Quick scan');
    expect(prompt).toContain('api/');
    expect(await db.latestRequestRun(request.id, 'assess')).toMatchObject({
      status: 'failed',
      error: 'rate limited',
    });
    expect(await db.latestAssessment(request.id)).toBeUndefined();
  });
});
