import { randomUUID } from 'node:crypto';
import { DatabaseSync, type SQLInputValue } from 'node:sqlite';
import {
  type AgentEvent,
  type Assessment,
  AssessmentJson,
  type ChangeRequest,
  NormalizedEvent,
  type Plan,
  PlanStatus,
  type PlanVariant,
  PlanVariantJson,
  type PlanWithVariants,
  type Project,
  type ProjectSettings,
  type RepoMap,
  RepoMapJson,
  type Run,
  RunKind,
  RunStatus,
  SetupStatus,
  type Task,
  type Terminal,
  TerminalKind,
  type Workspace,
} from '@studio/schema';
import { and, asc, desc, eq, gt, sql } from 'drizzle-orm';
import { drizzle, type SqliteRemoteDatabase } from 'drizzle-orm/sqlite-proxy';
import { migrate } from 'drizzle-orm/sqlite-proxy/migrator';
import {
  agentEvents,
  assessments,
  changeRequests,
  plans,
  planVariants,
  projects,
  repoMaps,
  runs,
  tasks,
  terminals,
  workspaces,
} from './tables';

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
      allowedCommands: [],
      editorCommand: null,
      terminalApp: null,
      agentCommand: null,
      createdAt: Date.now(),
    };
    await this.orm.insert(projects).values(project);
    return project;
  }

  async updateProjectSettings(id: string, settings: ProjectSettings): Promise<void> {
    await this.orm.update(projects).set(settings).where(eq(projects.id, id));
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

  // ── plans ──

  /** Saves a planner answer: a plan awaiting the user's choice and its variants, in the agent's order. */
  async addPlan(input: {
    projectId: string;
    requestId: string;
    runId: string;
    variants: PlanVariantJson[];
  }): Promise<PlanWithVariants> {
    const createdAt = Date.now();
    const plan: Plan = {
      id: randomUUID(),
      projectId: input.projectId,
      requestId: input.requestId,
      runId: input.runId,
      status: 'awaiting_choice',
      chosenVariantId: null,
      baseSha: null,
      integrationBranch: null,
      createdAt,
      approvedAt: null,
    };
    const variants: PlanVariant[] = input.variants.map((json, position) => ({
      id: randomUUID(),
      planId: plan.id,
      position,
      json,
      createdAt,
    }));
    await this.orm.insert(plans).values(plan);
    for (const v of variants) {
      await this.orm.insert(planVariants).values({ ...v, json: JSON.stringify(v.json) });
    }
    return { ...plan, variants };
  }

  async getPlan(id: string): Promise<PlanWithVariants | undefined> {
    const [row] = await this.orm.select().from(plans).where(eq(plans.id, id));
    return row && this.withVariants(row);
  }

  async latestPlan(requestId: string): Promise<PlanWithVariants | undefined> {
    const [row] = await this.orm
      .select()
      .from(plans)
      .where(eq(plans.requestId, requestId))
      .orderBy(desc(plans.createdAt))
      .limit(1);
    return row && this.withVariants(row);
  }

  async approvePlan(planId: string, variantId: string): Promise<void> {
    await this.orm
      .update(plans)
      .set({ status: 'approved', chosenVariantId: variantId, approvedAt: Date.now() })
      .where(eq(plans.id, planId));
  }

  /** Back to choosing: the approval is withdrawn, the variants stay. */
  async reopenPlan(planId: string): Promise<void> {
    await this.orm
      .update(plans)
      .set({ status: 'awaiting_choice', chosenVariantId: null, approvedAt: null })
      .where(eq(plans.id, planId));
  }

  async setPlanStatus(planId: string, status: PlanStatus): Promise<void> {
    await this.orm.update(plans).set({ status }).where(eq(plans.id, planId));
  }

  // ── plan tasks (started ones) ──

  async addTask(input: Omit<Task, 'id' | 'createdAt'>): Promise<Task> {
    const task: Task = { id: randomUUID(), createdAt: Date.now(), ...input };
    await this.orm.insert(tasks).values(task);
    return task;
  }

  async getTask(id: string): Promise<Task | undefined> {
    const [row] = await this.orm.select().from(tasks).where(eq(tasks.id, id));
    return row;
  }

  async findTask(planId: string, key: string): Promise<Task | undefined> {
    const [row] = await this.orm
      .select()
      .from(tasks)
      .where(and(eq(tasks.planId, planId), eq(tasks.key, key)));
    return row;
  }

  async listTasks(planId: string): Promise<Task[]> {
    return this.orm.select().from(tasks).where(eq(tasks.planId, planId)).orderBy(asc(tasks.createdAt));
  }

  async setTaskWorktree(id: string, worktreePath: string | null): Promise<void> {
    await this.orm.update(tasks).set({ worktreePath }).where(eq(tasks.id, id));
  }

  async latestTaskRun(taskId: string): Promise<Run | undefined> {
    const [row] = await this.orm
      .select()
      .from(runs)
      .where(eq(runs.taskId, taskId))
      .orderBy(desc(runs.startedAt))
      .limit(1);
    return row && toRun(row);
  }

  private async withVariants(row: typeof plans.$inferSelect): Promise<PlanWithVariants> {
    const variants = await this.orm
      .select()
      .from(planVariants)
      .where(eq(planVariants.planId, row.id))
      .orderBy(asc(planVariants.position));
    return {
      ...row,
      status: PlanStatus.parse(row.status),
      variants: variants.map((v) => ({ ...v, json: PlanVariantJson.parse(JSON.parse(v.json)) })),
    };
  }

  // ── workspaces ──

  async addWorkspace(
    input: Omit<Workspace, 'id' | 'createdAt' | 'setupExitCode' | 'setupLog'>,
  ): Promise<Workspace> {
    const workspace: Workspace = {
      id: randomUUID(),
      ...input,
      setupExitCode: null,
      setupLog: '',
      createdAt: Date.now(),
    };
    await this.orm.insert(workspaces).values(workspace);
    return workspace;
  }

  async getWorkspace(id: string): Promise<Workspace | undefined> {
    const [row] = await this.orm.select().from(workspaces).where(eq(workspaces.id, id));
    return row && toWorkspace(row);
  }

  /** Newest first: the one just created is on top. */
  async listWorkspaces(projectId: string): Promise<Workspace[]> {
    const rows = await this.orm
      .select()
      .from(workspaces)
      .where(eq(workspaces.projectId, projectId))
      .orderBy(desc(workspaces.createdAt));
    return rows.map(toWorkspace);
  }

  async setWorkspaceSetup(
    id: string,
    setup: { status: SetupStatus; exitCode?: number | null; log?: string },
  ): Promise<void> {
    await this.orm
      .update(workspaces)
      .set({
        setupStatus: setup.status,
        ...(setup.exitCode !== undefined && { setupExitCode: setup.exitCode }),
        ...(setup.log !== undefined && { setupLog: setup.log }),
      })
      .where(eq(workspaces.id, id));
  }

  async removeWorkspace(id: string): Promise<void> {
    await this.orm.delete(workspaces).where(eq(workspaces.id, id));
  }

  /** Setup commands left `running` by a previous app session died with it. */
  async markInterruptedSetups(): Promise<void> {
    await this.orm
      .update(workspaces)
      .set({ setupStatus: 'interrupted' })
      .where(eq(workspaces.setupStatus, 'running'));
  }

  // ── workspace terminals ──

  async listTerminals(workspaceId: string): Promise<Terminal[]> {
    const rows = await this.orm
      .select()
      .from(terminals)
      .where(eq(terminals.workspaceId, workspaceId))
      .orderBy(asc(terminals.position));
    return rows.map(toTerminal);
  }

  async getTerminal(id: string): Promise<Terminal | undefined> {
    const [row] = await this.orm.select().from(terminals).where(eq(terminals.id, id));
    return row && toTerminal(row);
  }

  /** Adds a tab after the last one. */
  async addTerminal(input: {
    workspaceId: string;
    kind: TerminalKind;
    sessionId?: string | null;
  }): Promise<Terminal> {
    const [last] = await this.orm
      .select({ position: sql<number>`coalesce(max(${terminals.position}), -1)` })
      .from(terminals)
      .where(eq(terminals.workspaceId, input.workspaceId));
    const terminal: Terminal = {
      id: randomUUID(),
      workspaceId: input.workspaceId,
      kind: input.kind,
      position: (last?.position ?? -1) + 1,
      sessionId: input.sessionId ?? null,
      createdAt: Date.now(),
    };
    await this.orm.insert(terminals).values(terminal);
    return terminal;
  }

  async setTerminalSession(id: string, sessionId: string | null): Promise<void> {
    await this.orm.update(terminals).set({ sessionId }).where(eq(terminals.id, id));
  }

  async removeTerminal(id: string): Promise<void> {
    await this.orm.delete(terminals).where(eq(terminals.id, id));
  }

  // ── runs ──

  async startRun(input: {
    projectId: string;
    requestId?: string;
    taskId?: string;
    kind: RunKind;
    agentId: string;
  }): Promise<Run> {
    const run: Run = {
      id: randomUUID(),
      ...input,
      requestId: input.requestId ?? null,
      taskId: input.taskId ?? null,
      status: 'running',
      error: null,
      costUsd: null,
      beforeSha: null,
      afterSha: null,
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

  /** Worktree heads of an implement run (SPEC §3.1). */
  async setRunShas(id: string, shas: { beforeSha?: string | null; afterSha?: string | null }): Promise<void> {
    await this.orm.update(runs).set(shas).where(eq(runs.id, id));
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

function toTerminal(row: typeof terminals.$inferSelect): Terminal {
  return { ...row, kind: TerminalKind.parse(row.kind) };
}

function toWorkspace(row: typeof workspaces.$inferSelect): Workspace {
  return { ...row, setupStatus: SetupStatus.parse(row.setupStatus) };
}

function toRun(row: typeof runs.$inferSelect): Run {
  return { ...row, kind: RunKind.parse(row.kind), status: RunStatus.parse(row.status) };
}
