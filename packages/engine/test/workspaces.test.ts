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
import { type Db, openDb } from '@studio/db';
import { git } from '@studio/git';
import type { Project, StudioEvent, Workspace } from '@studio/schema';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { cleanLog, WorkspaceError, Workspaces } from '../src';

const migrations = fileURLToPath(new URL('../../db/drizzle', import.meta.url));

describe('Workspaces', () => {
  let tmp: string;
  let repo: string;
  let root: string;
  let db: Db;
  let project: Project;
  let events: StudioEvent[];
  let workspaces: Workspaces;

  const withSetup = async (setupCommand: string | null) => {
    await db.updateProjectSettings(project.id, {
      setupCommand,
      agentCommand: null,
      editorCommand: null,
      terminalApp: null,
    });
    project = (await db.getProject(project.id)) as Project;
  };

  /** Waits until the setup command of the workspace has finished. */
  const settled = async (id: string): Promise<Workspace> => {
    for (let i = 0; i < 200; i++) {
      const ws = await db.getWorkspace(id);
      if (ws && ws.setupStatus !== 'running') return ws;
      await new Promise((r) => setTimeout(r, 25));
    }
    throw new Error('setup did not finish');
  };

  beforeEach(async () => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-wss-')));
    repo = join(tmp, 'repo');
    root = join(tmp, 'worktrees');
    mkdirSync(repo);
    await git(repo, ['init', '-q', '-b', 'main']);
    await git(repo, ['config', 'user.name', 't']);
    await git(repo, ['config', 'user.email', 't@t']);
    writeFileSync(join(repo, 'README.md'), '# demo\n');
    writeFileSync(join(repo, '.gitignore'), '.env\n');
    writeFileSync(join(repo, '.env'), 'SECRET=1\n');
    await git(repo, ['add', '-A']);
    await git(repo, ['commit', '-qm', 'init']);

    db = await openDb(join(tmp, 'studio.db'), migrations);
    project = await db.addProject({ name: 'Arena', repoPath: repo, defaultBranch: 'main' });
    events = [];
    workspaces = new Workspaces({ db, notify: (e) => events.push(e), worktreesRoot: root });
  });
  afterEach(() => {
    workspaces.killAll();
    db.close();
    rmSync(tmp, { recursive: true, force: true });
  });

  it('creates a branch and worktree from main and runs setup there with the workspace env', async () => {
    await withSetup('cp "$STUDIO_ROOT_PATH/.env" .env && echo "ready on $STUDIO_BRANCH"');
    const { workspace, fetchError } = await workspaces.create(project, 'Починить логин');

    expect(fetchError).toBeNull();
    expect(workspace).toMatchObject({
      name: 'Починить логин',
      slug: 'pochinit-login',
      branch: 'studio/ws/pochinit-login',
      path: join(root, 'arena', 'ws', 'pochinit-login'),
      baseRef: 'main',
      baseSha: await git(repo, ['rev-parse', 'main']),
      setupStatus: 'running',
    });
    expect(await git(workspace.path, ['rev-parse', '--abbrev-ref', 'HEAD'])).toBe('studio/ws/pochinit-login');

    const done = await settled(workspace.id);
    expect(done).toMatchObject({ setupStatus: 'done', setupExitCode: 0 });
    expect(done.setupLog).toContain('ready on studio/ws/pochinit-login');
    expect(readFileSync(join(workspace.path, '.env'), 'utf8')).toBe('SECRET=1\n');
    expect(events.every((e) => e.type === 'workspace' && e.workspaceId === workspace.id)).toBe(true);

    // The user's checkout is untouched.
    expect(await git(repo, ['rev-parse', '--abbrev-ref', 'HEAD'])).toBe('main');
    expect(await git(repo, ['status', '--porcelain'])).toBe('');
  });

  it('gives a second workspace with the same name its own slug, and skips setup when there is none', async () => {
    const first = await workspaces.create(project, 'fix');
    const second = await workspaces.create(project, 'fix');
    expect(first.workspace.setupStatus).toBe('none');
    expect(second.workspace.slug).toBe('fix-2');
    await expect(workspaces.create(project, '???')).rejects.toBeInstanceOf(WorkspaceError);
  });

  it('keeps the output of a failed setup and runs it again on request', async () => {
    await withSetup("echo step one; printf 'downloading 10%%\\rdownloading 100%%\\n'; echo oops >&2; exit 3");
    const { workspace } = await workspaces.create(project, 'w');
    const failed = await settled(workspace.id);
    expect(failed).toMatchObject({ setupStatus: 'failed', setupExitCode: 3 });
    expect(failed.setupLog).toBe('step one\ndownloading 100%\noops\n');

    await withSetup('echo second try');
    await workspaces.retrySetup(project, failed);
    const done = await settled(workspace.id);
    expect(done.setupStatus).toBe('done');
    expect(workspaces.setupLog(done)).toBe('second try\n');
  });

  it('warns about uncommitted work, then removes the worktree and keeps the branch', async () => {
    await withSetup('sleep 30');
    const { workspace } = await workspaces.create(project, 'w');
    writeFileSync(join(workspace.path, 'draft.txt'), 'wip\n');
    expect(await workspaces.changes(workspace)).toEqual({
      uncommitted: ['draft.txt'],
      unpushed: 0,
      missing: false,
    });

    // Removing while setup still runs stops it rather than waiting 30 seconds.
    const started = Date.now();
    await workspaces.remove(project, workspace);
    expect(Date.now() - started).toBeLessThan(5000);
    expect(existsSync(workspace.path)).toBe(false);
    expect(await db.getWorkspace(workspace.id)).toBeUndefined();
    expect(await git(repo, ['branch', '--list', 'studio/ws/w', '--format=%(refname:short)'])).toBe(
      'studio/ws/w',
    );
  });

  it('turns terminal output into plain text', () => {
    expect(cleanLog('\x1b[32mok\x1b[0m\r\n50%\r100%\n\x1b]0;title\x07done')).toBe('ok\n100%\ndone');
  });
});
