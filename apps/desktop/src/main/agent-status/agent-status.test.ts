import {
  chmodSync,
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
import { type Db, openDb } from '@studio/db';
import type { AgentStatus, StudioEvent } from '@studio/schema';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PtyHost } from '../terminals/pty-host';
import { Terminals } from '../terminals/terminals';
import { HOOK_EVENTS, HookServer, writeClaudeHookSettings } from './hook-server';
import { startAgentStatus } from './index';
import { AgentStatuses, type StatusChange } from './statuses';
import { WorkspaceStatuses } from './workspace-statuses';

const migrations = fileURLToPath(new URL('../../../../../packages/db/drizzle', import.meta.url));

describe('HookServer', () => {
  let server: HookServer;
  const events: [string, unknown][] = [];
  beforeEach(() => {
    events.length = 0;
    server = new HookServer((id, payload) => events.push([id, payload]));
  });
  afterEach(() => server.close());

  it('takes hooks only with a known token, and only on POST /hook', async () => {
    const port = await server.start();
    const token = server.tokenFor('t1');
    const post = (auth: string, path = '/hook') =>
      fetch(`http://127.0.0.1:${port}${path}`, {
        method: 'POST',
        headers: { authorization: auth, 'content-type': 'application/json' },
        body: JSON.stringify({ hook_event_name: 'Stop' }),
      });

    const ok = await post(`Bearer ${token}`);
    expect(ok.status).toBe(200);
    expect(await ok.json()).toEqual({});
    expect((await post('Bearer nope')).status).toBe(403);
    expect((await post(`Bearer ${token}`, '/other')).status).toBe(404);
    expect(events).toEqual([['t1', { hook_event_name: 'Stop' }]]);

    // A new start of the same terminal replaces its token.
    server.tokenFor('t1');
    expect((await post(`Bearer ${token}`)).status).toBe(403);
  });

  it('writes a settings file with HTTP hooks that read the token from the environment', () => {
    const dir = mkdtempSync(join(tmpdir(), 'studio-hooks-'));
    const file = writeClaudeHookSettings(dir, 4321);
    const settings = JSON.parse(readFileSync(file, 'utf8'));
    expect(Object.keys(settings)).toEqual(['hooks']);
    expect(Object.keys(settings.hooks)).toEqual(HOOK_EVENTS);
    expect(settings.hooks.Stop[0].hooks[0]).toEqual({
      type: 'http',
      url: 'http://127.0.0.1:4321/hook',
      headers: { Authorization: 'Bearer $STUDIO_HOOK_TOKEN' },
      allowedEnvVars: ['STUDIO_HOOK_TOKEN'],
      timeout: 5,
    });
    rmSync(dir, { recursive: true, force: true });
  });
});

describe('AgentStatuses', () => {
  let changes: StatusChange[];
  let statuses: AgentStatuses;
  const seq = () => changes.map((c) => c.status);
  beforeEach(() => {
    vi.useFakeTimers();
    changes = [];
    statuses = new AgentStatuses((c) => changes.push(c), 1000);
  });
  afterEach(() => vi.useRealTimers());

  it('follows Claude hooks: prompt, permission, tool, stop', () => {
    statuses.started('a', { hooks: true });
    statuses.hook('a', { hook_event_name: 'UserPromptSubmit' });
    statuses.hook('a', { hook_event_name: 'PermissionRequest' });
    statuses.hook('a', { hook_event_name: 'PostToolUse' });
    statuses.hook('a', { hook_event_name: 'PostToolUse' });
    statuses.hook('a', { hook_event_name: 'Notification', notification_type: 'elicitation_dialog' });
    statuses.hook('a', { hook_event_name: 'UserPromptSubmit' });
    statuses.hook('a', { hook_event_name: 'Stop' });
    // A minute after Stop Claude says it is idle: no second "finished".
    statuses.hook('a', { hook_event_name: 'Notification', notification_type: 'idle_prompt' });
    statuses.exited('a');
    expect(seq()).toEqual([
      'idle',
      'working',
      'needs_input',
      'working',
      'needs_input',
      'working',
      'idle',
      'exited',
    ]);
  });

  it('marks a silent working agent stalled and wakes it on output', () => {
    statuses.started('a', { hooks: true });
    statuses.hook('a', { hook_event_name: 'UserPromptSubmit' });
    vi.advanceTimersByTime(600);
    statuses.output('a'); // Output resets the silence clock.
    vi.advanceTimersByTime(600);
    expect(statuses.get('a')).toBe('working');
    vi.advanceTimersByTime(500);
    expect(statuses.get('a')).toBe('stalled');
    statuses.output('a');
    expect(statuses.get('a')).toBe('working');
    // Output of an idle Claude (e.g. typing) does not make it working: only hooks do.
    statuses.hook('a', { hook_event_name: 'Stop' });
    statuses.output('a');
    expect(statuses.get('a')).toBe('idle');
  });

  it('judges an agent without hooks by its output', () => {
    statuses.started('b', { hooks: false });
    statuses.output('b');
    expect(statuses.get('b')).toBe('working');
    vi.advanceTimersByTime(1000);
    expect(seq()).toEqual(['idle', 'working', 'stalled']);
  });
});

describe('WorkspaceStatuses', () => {
  let events: StudioEvent[];
  let badge: number;
  let ws: WorkspaceStatuses;
  const owner = (id: string) => ({ workspaceId: id, projectId: 'p', title: `p / ${id}` });
  beforeEach(() => {
    events = [];
    badge = 0;
    ws = new WorkspaceStatuses({
      notify: (e) => events.push(e),
      setBadge: (n) => {
        badge = n;
      },
      onSession: () => {},
    });
  });

  it('marks unseen questions and finished turns, notifies, and clears them when viewed', () => {
    ws.track('t1', owner('w1'), { hooks: true });
    ws.track('t2', owner('w2'), { hooks: true });
    ws.view('w2');
    ws.hook('t1', { hook_event_name: 'PermissionRequest' });
    ws.hook('t2', { hook_event_name: 'UserPromptSubmit' });
    ws.hook('t2', { hook_event_name: 'Stop' }); // Seen as it happens: no mark, no notification.

    expect(ws.list()).toEqual([
      { workspaceId: 'w1', projectId: 'p', status: 'needs_input', attention: true },
      { workspaceId: 'w2', projectId: 'p', status: 'idle', attention: false },
    ]);
    expect(badge).toBe(1);
    expect(
      events.filter((e) => e.type === 'agent' && e.notify).map((e) => e.type === 'agent' && e.notify),
    ).toEqual(['needs_input']);

    ws.view('w1');
    expect(ws.list()[0]?.attention).toBe(false);
    expect(badge).toBe(1); // Still asking: counted until answered.
    ws.hook('t1', { hook_event_name: 'PostToolUse' });
    expect(badge).toBe(0);
  });
});

describe('agent in a terminal', () => {
  let tmp: string;
  let db: Db;
  let host: PtyHost;
  let stop: () => void;
  const saved = { PATH: process.env.PATH, SHELL: process.env.SHELL, STUDIO_HOME: process.env.STUDIO_HOME };

  beforeEach(async () => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-agent-')));
    db = await openDb(join(tmp, 'studio.db'), migrations);
  });
  afterEach(() => {
    stop?.();
    host?.killAll();
    db.close();
    Object.assign(process.env, saved);
    rmSync(tmp, { recursive: true, force: true });
  });

  it('a fake claude in a pty reports its turn through the hooks, and its new session id is kept', async () => {
    // A stand-in claude: reads --settings, then posts a scenario of hooks with its token, as Claude does.
    const bin = join(tmp, 'bin');
    mkdirSync(bin);
    writeFileSync(
      join(bin, 'claude'),
      `#!/usr/bin/env node
const args = process.argv.slice(2);
const settings = JSON.parse(require('node:fs').readFileSync(args[args.indexOf('--settings') + 1], 'utf8'));
const url = settings.hooks.Stop[0].hooks[0].url;
const post = (body) => fetch(url, { method: 'POST', headers: { Authorization: 'Bearer ' + process.env.STUDIO_HOOK_TOKEN }, body: JSON.stringify(body) });
(async () => {
  for (const e of ['UserPromptSubmit', 'PermissionRequest', 'PostToolUse', 'Stop']) {
    await post({ hook_event_name: e, session_id: e === 'Stop' ? 'after-clear' : args[args.indexOf('--session-id') + 1] });
  }
  setTimeout(() => process.exit(0), 100);
})();
`,
    );
    chmodSync(join(bin, 'claude'), 0o755);
    process.env.PATH = `${bin}:${process.env.PATH}`;
    process.env.SHELL = '/nonexistent';
    process.env.STUDIO_HOME = join(tmp, 'home');

    const seen: AgentStatus[] = [];
    const agentStatus = await startAgentStatus({
      notify: () => {},
      setBadge: () => {},
      onSession: (id, session) => void db.setTerminalSession(id, session),
    });
    stop = agentStatus.stop;
    const { statuses } = agentStatus;
    host = new PtyHost({
      data: (c) => statuses.statuses.output(c.terminalId),
      exit: (id) => statuses.statuses.exited(id),
    });
    const record = statuses.statuses;
    const original = record.hook.bind(record);
    const terminals = new Terminals({ db, host, notify: () => {}, hooks: agentStatus.hooks });

    const repo = join(tmp, 'repo');
    mkdirSync(repo);
    const project = await db.addProject({ name: 'arena', repoPath: repo, defaultBranch: 'main' });
    const workspace = await db.addWorkspace({
      projectId: project.id,
      name: 'w',
      slug: 'w',
      branch: 'studio/ws/w',
      path: repo,
      baseRef: 'main',
      baseSha: 'abc',
      setupStatus: 'none',
    });
    await terminals.ensureDefaults(workspace.id);
    const [agent] = await db.listTerminals(workspace.id);
    if (!agent) throw new Error('no agent tab');
    record.hook = (id, payload) => {
      original(id, payload);
      seen.push(record.get(id) as AgentStatus);
    };

    await terminals.start({ project, workspace, terminal: agent }, { cols: 80, rows: 24 });
    const started = (await db.getTerminal(agent.id))?.sessionId;
    await vi.waitFor(() => expect(statuses.list()[0]?.status).toBe('exited'), 5000);

    expect(seen).toEqual(['working', 'needs_input', 'working', 'idle']);
    expect(started).toMatch(/^[0-9a-f-]{36}$/);
    expect((await db.getTerminal(agent.id))?.sessionId).toBe('after-clear');
  });
});
