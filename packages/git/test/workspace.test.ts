import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  createWorktree,
  freeWorkspaceSlug,
  freshBase,
  git,
  resolveCommit,
  workspaceLayout,
  worktreeChanges,
} from '../src';

describe('workspaces', () => {
  let tmp: string;
  let repo: string;
  let root: string;

  const commit = async (cwd: string, file: string, text: string) => {
    writeFileSync(join(cwd, file), text);
    await git(cwd, ['add', '-A']);
    await git(cwd, ['commit', '-qm', `edit ${file}`]);
  };

  beforeEach(async () => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-ws-')));
    repo = join(tmp, 'repo');
    root = join(tmp, 'worktrees');
    mkdirSync(repo);
    mkdirSync(root);
    await git(repo, ['init', '-q', '-b', 'main']);
    await git(repo, ['config', 'user.name', 't']);
    await git(repo, ['config', 'user.email', 't@t']);
    await commit(repo, 'README.md', '# demo\n');
  });
  afterEach(() => rmSync(tmp, { recursive: true, force: true }));

  it('lays a workspace out under <project>/ws/<slug> on studio/ws/<slug>', () => {
    expect(workspaceLayout({ root, projectName: 'Arena', slug: 'fix-login' })).toEqual({
      path: join(root, 'arena', 'ws', 'fix-login'),
      branch: 'studio/ws/fix-login',
    });
  });

  it('picks a slug no workspace, folder or branch uses yet', async () => {
    const pick = (name: string, taken: string[] = []) =>
      freeWorkspaceSlug({ repo, root, projectName: 'arena', name, taken: new Set(taken) });
    expect(await pick('Починить логин')).toBe('pochinit-login');
    expect(await pick('Починить логин', ['pochinit-login'])).toBe('pochinit-login-2');

    // A branch left from a deleted workspace is not picked up again.
    await git(repo, ['branch', 'studio/ws/fix']);
    expect(await pick('fix')).toBe('fix-2');
    mkdirSync(workspaceLayout({ root, projectName: 'arena', slug: 'fix-2' }).path, { recursive: true });
    expect(await pick('fix')).toBe('fix-3');

    expect(await pick('!!!')).toBeNull();
  });

  describe('fresh base', () => {
    let origin: string;
    let other: string;

    beforeEach(async () => {
      origin = join(tmp, 'origin.git');
      other = join(tmp, 'other');
      await git(tmp, ['clone', '-q', '--bare', repo, origin]);
      await git(repo, ['remote', 'add', 'origin', origin]);
      await git(repo, ['fetch', '-q', 'origin']);
      // Someone else pushes to main; the local main does not know yet.
      await git(tmp, ['clone', '-q', origin, other]);
      await git(other, ['config', 'user.name', 'o']);
      await git(other, ['config', 'user.email', 'o@o']);
      await commit(other, 'news.md', 'new\n');
      await git(other, ['push', '-q', 'origin', 'main']);
    });

    it('starts from the newest origin/main and leaves the local main alone', async () => {
      const localMain = await resolveCommit(repo, 'main');
      const base = await freshBase(repo, 'main');
      expect(base).toEqual({
        ref: 'origin/main',
        sha: await resolveCommit(other, 'HEAD'),
        fetchError: null,
      });
      expect(await resolveCommit(repo, 'main')).toBe(localMain);
    });

    it('falls back to the last known origin/main when the fetch fails', async () => {
      const known = await resolveCommit(repo, 'origin/main');
      await git(repo, ['remote', 'set-url', 'origin', join(tmp, 'gone.git')]);
      const base = await freshBase(repo, 'main');
      expect(base.ref).toBe('origin/main');
      expect(base.sha).toBe(known);
      expect(base.fetchError).toBeTruthy();
    });
  });

  it('uses the local branch when there is no remote', async () => {
    expect(await freshBase(repo, 'main')).toEqual({
      ref: 'main',
      sha: await resolveCommit(repo, 'main'),
      fetchError: null,
    });
  });

  it('reports uncommitted files and commits made since the base', async () => {
    const base = await resolveCommit(repo, 'main');
    const { path, branch } = workspaceLayout({ root, projectName: 'arena', slug: 'w' });
    await createWorktree({ repo, path, branch, base: 'main' });
    expect(await worktreeChanges(path, base)).toEqual({ uncommitted: [], unpushed: 0 });

    await commit(path, 'a.txt', 'a\n');
    writeFileSync(join(path, 'README.md'), '# changed\n');
    writeFileSync(join(path, 'new file.txt'), 'n\n');
    await git(path, ['mv', 'a.txt', 'b.txt']);
    const changes = await worktreeChanges(path, base);
    expect(changes.unpushed).toBe(1);
    expect(changes.uncommitted.sort()).toEqual(['README.md', 'b.txt', 'new file.txt']);
  });
});
