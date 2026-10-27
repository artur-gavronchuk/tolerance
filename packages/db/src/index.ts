import { randomUUID } from 'node:crypto';
import { DatabaseSync, type SQLInputValue } from 'node:sqlite';
import {
  type AgentEvent,
  type Assessment,
  AssessmentJson,
  type ChangeRequest,
  NormalizedEvent,
  type Project,
  type RepoMap,
  RepoMapJson,
  type Run,
  RunKind,
  RunStatus,
} from '@studio/schema';
import { and, asc, desc, eq, gt, sql } from 'drizzle-orm';
import { drizzle, type SqliteRemoteDatabase } from 'drizzle-orm/sqlite-proxy';
import { migrate } from 'drizzle-orm/sqlite-proxy/migrator';
import { agentEvents, assessments, changeRequests, projects, repoMaps, runs } from './tables';

export * as tables from './tables';

/**
 * Opens (or creates) the Studio database and applies pending migrations.
 * Uses Node's built-in SQLite through drizzle's proxy driver, so there is no native module
 * to rebuild for Electron (see docs/DECISIONS.md).
 */
export async function openDb(file: string, migrationsFolder: string): Promise<Db> {
  const sqlite = new DatabaseSync(file);
  sqlite.exec('PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;');

  const orm = drizzle(async (query, params, method) => {
    const stmt = sqlite.prepare(query);
    const args = params as SQLInputValue[];
    if (method === 'run') {
      stmt.run(...args);
      return { rows: [] };
    }
    stmt.setReturnArrays(true);
    if (method === 'get') return { rows: (stmt.get(...args) as unknown[] | undefined) ?? [] };
    return { rows: stmt.all(...args) };
  });

  await migrate(
    orm,
    async (queries) => {
      sqlite.exec('BEGIN');
      try {
        for (const q of queries) sqlite.exec(q);
        sqlite.exec('COMMIT');
      } catch (error) {
        sqlite.exec('ROLLBACK');
        throw error;
      }
    },
    { migrationsFolder },
  );

  return new Db(orm, () => sqlite.close());
}

type Orm = SqliteRemoteDatabase<Record<string, never>>;

export class Db {
  constructor(
    private readonly orm: Orm,
    readonly close: () => void,
  ) {}

  // ── projects ──

  async listProjects(): Promise<Project[]> {
    return this.orm.select().from(projects).orderBy(asc(projects.name));
  }

  async getProject(id: string): Promise<Project | undefined> {
    const [row] = await this.orm.select().from(projects).where(eq(projects.id, id));
    return row;
  }

  async findProjectByPath(repoPath: string): Promise<Project | undefined> {
    const [row] = await this.orm.select().from(projects).where(eq(projects.repoPath, repoPath));
    return row;
  }

  async addProject(input: { name: string; repoPath: string; defaultBranch: string }): Promise<Project> {
    const project: Project = {
      id: randomUUID(),
      ...input,
      testCommand: null,
      setupCommand: null,
      createdAt: Date.now(),
    };
    await this.orm.insert(projects).values(project);
    return project;
  }

  async removeProject(id: string): Promise<void> {
    await this.orm.delete(projects).where(eq(projects.id, id));
  }

  // ── repo maps ──

  async latestRepoMap(projectId: string): Promise<RepoMap | undefined> {
    const [row] = await this.orm
      .select()
      .from(repoMaps)
      .where(eq(repoMaps.projectId, projectId))
      .orderBy(desc(repoMaps.createdAt))
      .limit(1);
    return row && { ...row, json: RepoMapJson.parse(JSON.parse(row.json)) };
  }

  async addRepoMap(input: {
    projectId: string;
    commitSha: string | null;
    json: RepoMapJson;
  }): Promise<RepoMap> {
    const map: RepoMap = { id: randomUUID(), createdAt: Date.now(), ...input };
    await this.orm.insert(repoMaps).values({ ...map, json: JSON.stringify(map.json) });
    return map;
  }

  // ── change requests and their assessments ──

  async addRequest(input: { projectId: string; text: string }): Promise<ChangeRequest> {
    const request: ChangeRequest = { id: randomUUID(), ...input, createdAt: Date.now() };
    await this.orm.insert(changeRequests).values(request);
    return request;
  }

  async getRequest(id: string): Promise<ChangeRequest | undefined> {
    const [row] = await this.orm.select().from(changeRequests).where(eq(changeRequests.id, id));
    return row;
  }

  /** Newest first. */
  async listRequests(projectId: string): Promise<ChangeRequest[]> {
    return this.orm
      .select()
      .from(changeRequests)
      .where(eq(changeRequests.projectId, projectId))
      .orderBy(desc(changeRequests.createdAt));
  }

  async removeRequest(id: string): Promise<void> {
    await this.orm.delete(changeRequests).where(eq(changeRequests.id, id));
  }

  async addAssessment(input: {
    requestId: string;
    runId: string;
    json: AssessmentJson;
  }): Promise<Assessment> {
    const assessment: Assessment = { id: randomUUID(), createdAt: Date.now(), ...input };
    await this.orm.insert(assessments).values({ ...assessment, json: JSON.stringify(assessment.json) });
    return assessment;
  }

  async latestAssessment(requestId: string): Promise<Assessment | undefined> {
    const [row] = await this.orm
      .select()
      .from(assessments)
      .where(eq(assessments.requestId, requestId))
      .orderBy(desc(assessments.createdAt))
      .limit(1);
    return row && { ...row, json: AssessmentJson.parse(JSON.parse(row.json)) };
  }

  async latestRequestRun(requestId: string, kind: RunKind): Promise<Run | undefined> {
    const [row] = await this.orm
      .select()
      .from(runs)
      .where(and(eq(runs.requestId, requestId), eq(runs.kind, kind)))
      .orderBy(desc(runs.startedAt))
      .limit(1);
    return row && toRun(row);
  }

  // ── runs ──

  async startRun(input: {
    projectId: string;
    requestId?: string;
    kind: RunKind;
    agentId: string;
  }): Promise<Run> {
    const run: Run = {
      id: randomUUID(),
      ...input,
      requestId: input.requestId ?? null,
      status: 'running',
      error: null,
      costUsd: null,
      startedAt: Date.now(),
      endedAt: null,
    };
    await this.orm.insert(runs).values(run);
    return run;
  }

  async finishRun(
    id: string,
    result: { status: RunStatus; error?: string; costUsd?: number },
  ): Promise<void> {
    await this.orm
      .update(runs)
      .set({
        status: result.status,
        error: result.error ?? null,
        costUsd: result.costUsd ?? null,
        endedAt: Date.now(),
      })
      .where(eq(runs.id, id));
  }

  async latestRun(projectId: string, kind: RunKind): Promise<Run | undefined> {
    const [row] = await this.orm
      .select()
      .from(runs)
      .where(and(eq(runs.projectId, projectId), eq(runs.kind, kind)))
      .orderBy(desc(runs.startedAt))
      .limit(1);
    return row && toRun(row);
  }

  /** Runs left `running` by a previous app session can no longer finish. */
  async markInterruptedRuns(): Promise<void> {
    await this.orm
      .update(runs)
      .set({ status: 'interrupted', endedAt: Date.now() })
      .where(eq(runs.status, 'running'));
  }

  // ── agent events (append-only) ──

  async appendEvent(runId: string, event: NormalizedEvent): Promise<AgentEvent> {
    const [last] = await this.orm
      .select({ seq: sql<number>`coalesce(max(${agentEvents.seq}), 0)` })
      .from(agentEvents)
      .where(eq(agentEvents.runId, runId));
    const row: AgentEvent = { runId, seq: (last?.seq ?? 0) + 1, ts: Date.now(), event };
    await this.orm
      .insert(agentEvents)
      .values({ runId, seq: row.seq, ts: row.ts, payload: JSON.stringify(event) });
    return row;
  }

  async listEvents(runId: string, afterSeq = 0): Promise<AgentEvent[]> {
    const rows = await this.orm
      .select()
      .from(agentEvents)
      .where(and(eq(agentEvents.runId, runId), gt(agentEvents.seq, afterSeq)))
      .orderBy(asc(agentEvents.seq));
    return rows.map((r) => ({
      runId: r.runId,
      seq: r.seq,
      ts: r.ts,
      event: NormalizedEvent.parse(JSON.parse(r.payload)),
    }));
  }
}

function toRun(row: typeof runs.$inferSelect): Run {
  return { ...row, kind: RunKind.parse(row.kind), status: RunStatus.parse(row.status) };
}
