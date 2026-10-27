import { index, integer, primaryKey, real, sqliteTable, text } from 'drizzle-orm/sqlite-core';

export const projects = sqliteTable('projects', {
  id: text('id').primaryKey(),
  name: text('name').notNull(),
  repoPath: text('repo_path').notNull().unique(),
  defaultBranch: text('default_branch').notNull(),
  testCommand: text('test_command'),
  setupCommand: text('setup_command'),
  createdAt: integer('created_at').notNull(),
});

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
    kind: text('kind').notNull(),
    agentId: text('agent_id').notNull(),
    status: text('status').notNull(),
    error: text('error'),
    costUsd: real('cost_usd'),
    startedAt: integer('started_at').notNull(),
    endedAt: integer('ended_at'),
  },
  (t) => [
    index('runs_project_idx').on(t.projectId, t.kind, t.startedAt),
    index('runs_request_idx').on(t.requestId, t.startedAt),
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
