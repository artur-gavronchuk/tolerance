import { realpath } from 'node:fs/promises';
import { git } from './exec';

export * from './diff';
export { git } from './exec';
export * from './workspace';
export * from './worktree';

export class NotAGitRepoError extends Error {
  constructor(readonly path: string) {
    super(`Not a git repository: ${path}`);
  }
}

/** Resolves any path inside a working tree to the real path of its root. */
export async function repoRoot(path: string): Promise<string> {
  try {
    return await realpath(await git(path, ['rev-parse', '--show-toplevel']));
  } catch {
    throw new NotAGitRepoError(path);
  }
}

export async function currentBranch(repo: string): Promise<string | null> {
  const name = await git(repo, ['rev-parse', '--abbrev-ref', 'HEAD']).catch(() => '');
  return name && name !== 'HEAD' ? name : null;
}

export async function headSha(repo: string): Promise<string | null> {
  return git(repo, ['rev-parse', 'HEAD']).catch(() => null);
}

/** The remote's default branch when known, otherwise the current branch, otherwise "main". */
export async function defaultBranch(repo: string): Promise<string> {
  const remoteHead = await git(repo, [
    'symbolic-ref',
    '--quiet',
    '--short',
    'refs/remotes/origin/HEAD',
  ]).catch(() => '');
  if (remoteHead) return remoteHead.replace(/^origin\//, '');
  return (await currentBranch(repo)) ?? 'main';
}

/** Tracked and untracked-but-not-ignored files, relative to the repo root. */
export async function listFiles(repo: string): Promise<string[]> {
  const out = await git(repo, ['ls-files', '-z', '--cached', '--others', '--exclude-standard']);
  return [...new Set(out.split('\0').filter(Boolean))].sort();
}
