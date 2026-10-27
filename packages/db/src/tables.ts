import {
  type AnySQLiteColumn,
  index,
  integer,
  primaryKey,
  real,
  sqliteTable,
  text,
  uniqueIndex,
} from 'drizzle-orm/sqlite-core';

export const projects = sqliteTable('projects', {
  id: text('id').primaryKey(),
  name: text('name').notNull(),
  repoPath: text('repo_path').notNull().unique(),
  defaultBranch: text('default_branch').notNull(),
  testCommand: text('test_command'),
  setupCommand: text('setup_command'),
  allowedCommands: text('allowed_commands', { mode: 'json' }).$type<string[]>().notNull().default([]),
  editorCommand: text('editor_command'),
  terminalApp: text('terminal_app'),
  agentCommand: text('agent_command'),
  createdAt: integer('created_at').notNull(),
});

export const workspaces = sqliteTable(
  'workspaces',
  {
    id: text('id').primaryKey(),
    projectId: text('project_id')
      .notNull()
      .references(() => projects.id, { onDelete: 'cascade' }),
    name: text('name').notNull(),
    slug: text('slug').notNull(),
    branch: text('branch').notNull(),
    path: text('path').notNull(),
    baseRef: text('base_ref').notNull(),
    baseSha: text('base_sha').notNull(),
    setupStatus: text('setup_status').notNull(),
    setupExitCode: integer('setup_exit_code'),
    setupLog: text('setup_log').notNull().default(''),
    createdAt: integer('created_at').notNull(),
  },
  (t) => [uniqueIndex('workspaces_project_slug_idx').on(t.projectId, t.slug)],
);

export const terminals = sqliteTable(
  'terminals',
  {
    id: text('id').primaryKey(),
    workspaceId: text('workspace_id')
      .notNull()
      .references(() => workspaces.id, { onDelete: 'cascade' }),
    kind: text('kind').notNull(),
    position: integer('position').notNull(),
    sessionId: text('session_id'),
    createdAt: integer('created_at').notNull(),
  },
  (t) => [index('terminals_workspace_idx').on(t.workspaceId, t.position)],
);

export const repoMaps = sqliteTable(
  'repo_maps',
  {
    id: text('id').primaryKey(),
    projectId: text('project_id')
      .notNull()
      .references(() => projects.id, { onDelete: 'cascade' }),
    commitSha: text('commit_sha'),
    json: text('json').notNull(),
    createdAt: integer('created_at').notNull(),
  },
  (t) => [index('repo_maps_project_idx').on(t.projectId, t.createdAt)],
);

export const changeRequests = sqliteTable(
  'change_requests',
  {
    id: text('id').primaryKey(),
    projectId: text('project_id')
      .notNull()
      .references(() => projects.id, { onDelete: 'cascade' }),
    text: text('text').notNull(),
    createdAt: integer('created_at').notNull(),
  },
  (t) => [index('change_requests_project_idx').on(t.projectId, t.createdAt)],
);

export const runs = sqliteTable(
  'runs',
  {
    id: text('id').primaryKey(),
    projectId: text('project_id')
      .notNull()
      .references(() => projects.id, { onDelete: 'cascade' }),
    requestId: text('request_id').references(() => changeRequests.id, { onDelete: 'cascade' }),
    // Typed by hand: tasks → plans → runs reference each other.
    taskId: text('task_id').references((): AnySQLiteColumn => tasks.id, { onDelete: 'cascade' }),
    kind: text('kind').notNull(),
    agentId: text('agent_id').notNull(),
    status: text('status').notNull(),
    error: text('error'),
    costUsd: real('cost_usd'),
    beforeSha: text('before_sha'),
    afterSha: text('after_sha'),
    startedAt: integer('started_at').notNull(),
    endedAt: integer('ended_at'),
  },
  (t) => [
    index('runs_project_idx').on(t.projectId, t.kind, t.startedAt),
    index('runs_request_idx').on(t.requestId, t.startedAt),
    index('runs_task_idx').on(t.taskId, t.startedAt),
  ],
);

export const assessments = sqliteTable(
  'assessments',
  {
    id: text('id').primaryKey(),
    requestId: text('request_id')
      .notNull()
      .references(() => changeRequests.id, { onDelete: 'cascade' }),
    runId: text('run_id')
      .notNull()
      .references(() => runs.id, { onDelete: 'cascade' }),
    json: text('json').notNull(),
    createdAt: integer('created_at').notNull(),
  },
  (t) => [index('assessments_request_idx').on(t.requestId, t.createdAt)],
);

export const agentEvents = sqliteTable(
  'agent_events',
  {
    runId: text('run_id')
      .notNull()
      .references(() => runs.id, { onDelete: 'cascade' }),
    seq: integer('seq').notNull(),
    ts: integer('ts').notNull(),
    payload: text('payload').notNull(),
  },
  (t) => [primaryKey({ columns: [t.runId, t.seq] })],
);

export const plans = sqliteTable(
  'plans',
  {
    id: text('id').primaryKey(),
    projectId: text('project_id')
      .notNull()
      .references(() => projects.id, { onDelete: 'cascade' }),
    requestId: text('request_id')
      .notNull()
      .references(() => changeRequests.id, { onDelete: 'cascade' }),
    runId: text('run_id')
      .notNull()
      .references(() => runs.id, { onDelete: 'cascade' }),
    status: text('status').notNull(),
    chosenVariantId: text('chosen_variant_id'),
    baseSha: text('base_sha'),
    integrationBranch: text('integration_branch'),
    createdAt: integer('created_at').notNull(),
    approvedAt: integer('approved_at'),
  },
  (t) => [index('plans_request_idx').on(t.requestId, t.createdAt)],
);

export const planVariants = sqliteTable(
  'plan_variants',
  {
    id: text('id').primaryKey(),
    planId: text('plan_id')
      .notNull()
      .references(() => plans.id, { onDelete: 'cascade' }),
    position: integer('position').notNull(),
    json: text('json').notNull(),
    createdAt: integer('created_at').notNull(),
  },
  (t) => [index('plan_variants_plan_idx').on(t.planId, t.position)],
);

export const tasks = sqliteTable(
  'tasks',
  {
    id: text('id').primaryKey(),
    planId: text('plan_id')
      .notNull()
      .references(() => plans.id, { onDelete: 'cascade' }),
    key: text('key').notNull(),
    branch: text('branch').notNull(),
    worktreePath: text('worktree_path'),
    baseRef: text('base_ref').notNull(),
    createdAt: integer('created_at').notNull(),
  },
  (t) => [uniqueIndex('tasks_plan_key_idx').on(t.planId, t.key)],
);
