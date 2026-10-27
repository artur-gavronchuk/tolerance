import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  commitAll,
  createWorktree,
  diffFromMergeBase,
  git,
  removeWorktree,
  slugify,
  taskWorktreeLayout,
  UnsafeWorktreePathError,
} from '../src';

const identity = ['-c', 'user.name=t', '-c', 'user.email=t@t'];

describe('task worktrees', () => {
  let tmp: string;
  let repo: string;
  let root: string;

  beforeEach(async () => {
    tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-wt-')));
    repo = join(tmp, 'repo');
    root = join(tmp, 'worktrees');
    mkdirSync(repo);
    mkdirSync(root);
    await git(repo, ['init', '-q', '-b', 'main']);
    await git(repo, ['config', 'user.name', 't']);
    await git(repo, ['config', 'user.email', 't@t']);
    writeFileSync(join(repo, 'README.md'), '# demo\n');
    writeFileSync(join(repo, 'old.txt'), 'old\n');
    writeFileSync(join(repo, '.gitignore'), 'node_modules/\n');
    await git(repo, ['add', '-A']);
    await git(repo, ['commit', '-qm', 'init']);
  });
  afterEach(() => rmSync(tmp, { recursive: true, force: true }));

  const layout = (title = 'Добавить раздел в README') =>
    taskWorktreeLayout({
      root,
      projectName: 'My Repo',
      planId: '0123456789abcdef',
      taskKey: 'T1',
      taskTitle: title,
    });

  it('names paths and branches as in the spec', () => {
    expect(layout()).toEqual({
      path: join(root, 'my-repo', '0123456789abcdef', 'T1'),
      branch: 'studio/01234567/T1-dobavit-razdel-v-readme',
    });
    expect(slugify('Fix: API v2 / auth!!')).toBe('fix-api-v2-auth');
    expect(layout('???').branch).toBe('studio/01234567/T1');
  });

  it('creates a worktree on a new branch from the base and leaves the main checkout alone', async () => {
    const { path, branch } = layout();
    const mainHead = await git(repo, ['rev-parse', 'HEAD']);
    expect(await createWorktree({ repo, path, branch, base: 'main' })).toEqual({ path, created: true });

    expect(await git(path, ['rev-parse', '--abbrev-ref', 'HEAD'])).toBe(branch);
    expect(await git(path, ['rev-parse', 'HEAD'])).toBe(mainHead);
    expect(await createWorktree({ repo, path, branch, base: 'main' })).toEqual({ path, created: false });

    writeFileSync(join(path, 'README.md'), '# demo\n\n## Run\n');
    const { committed, sha } = await commitAll(path, 'studio: T1 Добавить раздел');
    expect(committed).toBe(true);
    expect(await git(path, ['log', '-1', '--format=%s'])).toBe('studio: T1 Добавить раздел');
    expect(sha).not.toBe(mainHead);

    expect(await git(repo, ['rev-parse', 'HEAD'])).toBe(mainHead);
    expect(await git(repo, ['status', '--porcelain'])).toBe('');
    expect(readFileSync(join(repo, 'README.md'), 'utf8')).toBe('# demo\n');
  });

  it('does not commit when nothing changed', async () => {
    const { path, branch } = layout();
    await createWorktree({ repo, path, branch, base: 'main' });
    const head = await git(path, ['rev-parse', 'HEAD']);
    expect(await commitAll(path, 'nothing')).toEqual({ sha: head, committed: false });
  });

  it('commits as Studio when git has no identity', async () => {
    await git(repo, ['config', '--unset', 'user.email']);
    const { path, branch } = layout();
    await createWorktree({ repo, path, branch, base: 'main' });
    writeFileSync(join(path, 'new.txt'), 'x\n');
    // The machine's own git identity is hidden for this test.
    process.env.GIT_CONFIG_GLOBAL = '/dev/null';
    process.env.GIT_CONFIG_NOSYSTEM = '1';
    try {
      expect((await commitAll(path, 'studio: T1 x')).committed).toBe(true);
      expect(await git(path, ['log', '-1', '--format=%ae'])).toBe('studio@localhost');
    } finally {
      delete process.env.GIT_CONFIG_GLOBAL;
      delete process.env.GIT_CONFIG_NOSYSTEM;
    }
  });

  it('diffs from the merge-base, with uncommitted and untracked files but not ignored ones', async () => {
    const { path, branch } = layout();
    await createWorktree({ repo, path, branch, base: 'main' });
    writeFileSync(join(path, 'README.md'), '# demo\n\n## Run\n');
    await git(path, [...identity, 'commit', '-qam', 'readme']);
    rmSync(join(path, 'old.txt'));
    writeFileSync(join(path, 'new.txt'), 'one\ntwo\n');
    mkdirSync(join(path, 'node_modules'));
    writeFileSync(join(path, 'node_modules', 'junk.js'), 'x');
    // main moves on after the task branched: its new commit is not part of the task diff.
    writeFileSync(join(repo, 'other.txt'), 'other\n');
    await git(repo, ['add', '-A']);
    await git(repo, ['commit', '-qm', 'other']);

    const diff = await diffFromMergeBase({ cwd: path, base: 'main' });
    expect(diff.files).toEqual([
      { path: 'README.md', status: 'modified', additions: 2, deletions: 0 },
      { path: 'new.txt', status: 'added', additions: 2, deletions: 0 },
      { path: 'old.txt', status: 'deleted', additions: 0, deletions: 1 },
    ]);
    expect(diff.patch).toContain('+## Run');
    expect(diff.patch).toContain('+++ b/new.txt');
    expect(diff.truncated).toBe(false);
    // The worktree's own index is untouched: new.txt is still untracked there.
    expect(await git(path, ['status', '--porcelain'])).toContain('?? new.txt');

    const short = await diffFromMergeBase({ cwd: path, base: 'main', maxPatchChars: 10 });
    expect(short).toMatchObject({ truncated: true, patch: diff.patch.slice(0, 10) });
  });

  it('diffs a branch whose worktree is gone, and reuses the branch when the worktree comes back', async () => {
    const { path, branch } = layout();
    await createWorktree({ repo, path, branch, base: 'main' });
    writeFileSync(join(path, 'new.txt'), 'x\n');
    const { sha } = await commitAll(path, 'studio: T1 x');

    await removeWorktree({ repo, root, path });
    expect(existsSync(path)).toBe(false);
    expect(await git(repo, ['worktree', 'list'])).not.toContain(path);
    expect(await git(repo, ['rev-parse', branch])).toBe(sha);

    const diff = await diffFromMergeBase({ cwd: repo, base: 'main', head: branch });
    expect(diff.files.map((f) => f.path)).toEqual(['new.txt']);

    await createWorktree({ repo, path, branch, base: 'main' });
    expect(await git(path, ['rev-parse', 'HEAD'])).toBe(sha);
  });

  it('removes a worktree with uncommitted changes and one whose folder was half deleted', async () => {
    const { path, branch } = layout();
    await createWorktree({ repo, path, branch, base: 'main' });
    writeFileSync(join(path, 'dirty.txt'), 'x');
    await removeWorktree({ repo, root, path });
    expect(existsSync(path)).toBe(false);

    await createWorktree({ repo, path, branch, base: 'main' });
    rmSync(join(path, '.git'));
    await removeWorktree({ repo, root, path });
    expect(existsSync(path)).toBe(false);

    // Already gone: nothing to do.
    await removeWorktree({ repo, root, path });
  });

  it('refuses to remove anything outside the worktrees root, symlinks included', async () => {
    await expect(removeWorktree({ repo, root, path: repo })).rejects.toBeInstanceOf(UnsafeWorktreePathError);
    await expect(removeWorktree({ repo, root, path: root })).rejects.toBeInstanceOf(UnsafeWorktreePathError);
    await expect(removeWorktree({ repo, root, path: join(root, '..', 'repo') })).rejects.toBeInstanceOf(
      UnsafeWorktreePathError,
    );
    const link = join(root, 'sneaky');
    symlinkSync(repo, link);
    await expect(removeWorktree({ repo, root, path: link })).rejects.toBeInstanceOf(UnsafeWorktreePathError);
    expect(existsSync(join(repo, 'README.md'))).toBe(true);
  });
});
