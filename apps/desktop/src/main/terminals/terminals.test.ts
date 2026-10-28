import { chmodSync, mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { openDb } from '@studio/db';
import type { Project, TerminalProcess, Workspace } from '@studio/schema';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { type PtyChunk, PtyHost } from './pty-host';
import { isClaude, Terminals, terminalCommand, terminalEnv } from './terminals';

describe('PtyHost', () => {
  let host: PtyHost;
  const chunks: PtyChunk[] = [];
  const exits: [string, TerminalProcess][] = [];

  const start = (id: string, script: string) =>
    host.start(id, {
      file: '/bin/sh',
      args: ['-c', script],
      cwd: '/tmp',
      env: { PATH: process.env.PATH, TERM: 'xterm-256color' },
      cols: 80,
      rows: 24,
    });

  afterEach(() => {
    host.killAll();
    chunks.length = 0;
    exits.length = 0;
  });

  host = new PtyHost({ data: (c) => chunks.push(c), exit: (id, p) => exits.push([id, p]) });

  it('streams output in numbered batches and keeps it for a later snapshot', async () => {
    expect(host.process('t1')).toEqual({ state: 'never', exitCode: null });
    start('t1', 'printf one; sleep 0.05; printf two; exit 3');
    expect(host.process('t1').state).toBe('running');
    await vi.waitFor(() => expect(exits).toHaveLength(1), 5000);

    expect(exits[0]).toEqual(['t1', { state: 'exited', exitCode: 3 }]);
    expect(chunks.map((c) => c.data).join('')).toBe('onetwo');
    expect(chunks.map((c) => c.seq)).toEqual(chunks.map((_, i) => i + 1));
    const snap = host.snapshot('t1');
    expect(snap).toMatchObject({
      data: 'onetwo',
      truncated: false,
      process: { state: 'exited', exitCode: 3 },
    });
    expect(snap.seq).toBe(chunks.at(-1)?.seq);
  });

  it('passes input and size to the program', async () => {
    start('t2', 'read line; stty size; echo "got $line"');
    host.resize('t2', 100, 30);
    host.write('t2', 'hello\r');
    await vi.waitFor(() => expect(exits).toHaveLength(1), 5000);
    const out = host.snapshot('t2').data;
    expect(out).toContain('30 100');
    expect(out).toContain('got hello');
  });

  it('hangs up a running program and its children', async () => {
    start('t3', 'sleep 30 & wait');
    host.kill('t3');
    await vi.waitFor(() => expect(exits).toHaveLength(1), 5000);
    expect(host.process('t3').state).toBe('exited');
    host.dispose('t3');
    expect(host.process('t3').state).toBe('never');
  });

  it('starts again in the same tab after the program ended, keeping the old output', async () => {
    start('t4', 'printf first');
    await vi.waitFor(() => expect(exits).toHaveLength(1), 5000);
    start('t4', 'printf second');
    await vi.waitFor(() => expect(exits).toHaveLength(2), 5000);
    expect(host.snapshot('t4').data).toBe('firstsecond');
  });
});

describe('terminal commands', () => {
  it('starts Claude with the session id through the login shell', () => {
    expect(
      terminalCommand({ kind: 'agent', agentCommand: null, sessionId: 'abc', shell: '/bin/zsh' }),
    ).toEqual({
      file: '/bin/zsh',
      args: ['-i', '-l', '-c', 'exec claude --session-id abc'],
    });
    expect(
      terminalCommand({
        kind: 'agent',
        agentCommand: 'claude --model opus',
        sessionId: 'abc',
        shell: '/bin/sh',
      }),
    ).toEqual({ file: '/bin/sh', args: ['-i', '-c', 'exec claude --model opus --session-id abc'] });
  });

  it('continues a stored Claude session with --resume', () => {
    expect(
      terminalCommand({
        kind: 'agent',
        agentCommand: null,
        sessionId: 'abc',
        resume: true,
        hookSettings: '/h/my hooks.json',
        shell: '/bin/zsh',
      }).args.at(-1),
    ).toBe("exec claude --settings '/h/my hooks.json' --resume abc");
  });

  it('runs other agents as they are, and shells as interactive login shells', () => {
    expect(isClaude('/opt/homebrew/bin/claude')).toBe(true);
    expect(isClaude('codex')).toBe(false);
    expect(
      terminalCommand({ kind: 'agent', agentCommand: 'codex', sessionId: null, shell: '/bin/bash' }),
    ).toEqual({
      file: '/bin/bash',
      args: ['-i', '-l', '-c', 'exec codex'],
    });
    expect(
      terminalCommand({ kind: 'shell', agentCommand: null, sessionId: null, shell: '/bin/zsh' }).args,
    ).toEqual(['-i', '-l']);
  });

  it('gives the terminal a clean environment that knows the workspace', () => {
    const env = terminalEnv(
      { PATH: '/bin', ELECTRON_RUN_AS_NODE: '1', CLAUDECODE: '1', HOME: '/h' },
      {
        project: { repoPath: '/repo' } as Project,
        workspace: { path: '/w', name: 'Логин', branch: 'studio/ws/login' } as Workspace,
      },
    );
    expect(env).toMatchObject({
      PATH: '/bin',
      HOME: '/h',
      TERM: 'xterm-256color',
      COLORTERM: 'truecolor',
      LANG: 'en_US.UTF-8',
      STUDIO_ROOT_PATH: '/repo',
      STUDIO_WORKSPACE_PATH: '/w',
      STUDIO_BRANCH: 'studio/ws/login',
    });
    expect(env.ELECTRON_RUN_AS_NODE).toBeUndefined();
    expect(env.CLAUDECODE).toBeUndefined();
  });
});

describe('resuming an agent session', () => {
  it('starts a new session when Claude has nothing to resume, and keeps a session that resumes', async () => {
    const tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-resume-')));
    const saved = { PATH: process.env.PATH, SHELL: process.env.SHELL };
    // A stand-in claude that knows one session: "known".
    const bin = join(tmp, 'bin');
    mkdirSync(bin);
    writeFileSync(
      join(bin, 'claude'),
      `#!/bin/sh
case "$*" in
  *"--resume known"*) echo "resumed known"; sleep 30 ;;
  *--resume*) echo "No conversation found with session ID"; exit 1 ;;
  *) echo "fresh session $*"; sleep 30 ;;
esac
`,
    );
    chmodSync(join(bin, 'claude'), 0o755);
    process.env.PATH = `${bin}:${saved.PATH}`;
    process.env.SHELL = '/nonexistent';

    const db = await openDb(
      join(tmp, 'studio.db'),
      fileURLToPath(new URL('../../../../../packages/db/drizzle', import.meta.url)),
    );
    let terminals: Terminals | undefined;
    const host = new PtyHost({ data: () => {}, exit: (id, p) => void terminals?.exited(id, p.exitCode) });
    terminals = new Terminals({ db, host, notify: () => {} });
    try {
      const project = await db.addProject({ name: 'arena', repoPath: tmp, defaultBranch: 'main' });
      const workspace = await db.addWorkspace({
        projectId: project.id,
        name: 'w',
        slug: 'w',
        branch: 'studio/ws/w',
        path: tmp,
        baseRef: 'main',
        baseSha: 'abc',
        setupStatus: 'none',
      });
      await terminals.ensureDefaults(workspace.id);
      const agentTab = async () => (await db.listTerminals(workspace.id))[0];
      const size = { cols: 80, rows: 24 };

      // A session id Claude never wrote (nothing was typed after the restart that lost it).
      const first = await agentTab();
      if (!first) throw new Error('no agent tab');
      await db.setTerminalSession(first.id, 'gone');
      await terminals.start({ project, workspace, terminal: (await agentTab()) ?? first }, size, {
        resume: true,
      });
      await vi.waitFor(() => expect(host.snapshot(first.id).data).toContain('fresh session'), 5000);
      const fresh = (await agentTab())?.sessionId;
      expect(fresh).toMatch(/^[0-9a-f-]{36}$/);
      expect(host.snapshot(first.id).data).toContain(`fresh session --session-id ${fresh}`);
      host.dispose(first.id);

      await db.setTerminalSession(first.id, 'known');
      await terminals.start({ project, workspace, terminal: (await agentTab()) ?? first }, size, {
        resume: true,
      });
      await vi.waitFor(() => expect(host.snapshot(first.id).data).toContain('resumed known'), 5000);
      expect((await agentTab())?.sessionId).toBe('known');
    } finally {
      host.killAll();
      db.close();
      Object.assign(process.env, saved);
      rmSync(tmp, { recursive: true, force: true });
    }
  });
});
