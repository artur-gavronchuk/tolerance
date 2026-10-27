import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { currentBranch, defaultBranch, git, headSha, listFiles, NotAGitRepoError, repoRoot } from '../src';

describe('git helpers', () => {
  let dir: string;

  beforeEach(async () => {
    dir = realpathSync(mkdtempSync(join(tmpdir(), 'studio-git-')));
    await git(dir, ['init', '-q', '-b', 'main']);
    mkdirSync(join(dir, 'src'));
    writeFileSync(join(dir, 'src', 'a.ts'), 'export {}');
    writeFileSync(join(dir, '.gitignore'), 'ignored.txt\n');
    writeFileSync(join(dir, 'ignored.txt'), 'x');
  });
  afterEach(() => rmSync(dir, { recursive: true, force: true }));

  it('finds the root from a subdirectory and rejects plain folders', async () => {
    expect(await repoRoot(join(dir, 'src'))).toBe(dir);
    const plain = mkdtempSync(join(tmpdir(), 'studio-plain-'));
    await expect(repoRoot(plain)).rejects.toBeInstanceOf(NotAGitRepoError);
    rmSync(plain, { recursive: true });
  });

  it('works before the first commit', async () => {
    expect(await headSha(dir)).toBeNull();
    expect(await defaultBranch(dir)).toBe('main');
  });

  it('lists tracked and untracked files without ignored ones', async () => {
    await git(dir, ['add', 'src/a.ts']);
    await git(dir, ['-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'init']);
    expect(await listFiles(dir)).toEqual(['.gitignore', 'src/a.ts']);
    expect(await currentBranch(dir)).toBe('main');
    expect(await headSha(dir)).toMatch(/^[0-9a-f]{40}$/);
  });
});
