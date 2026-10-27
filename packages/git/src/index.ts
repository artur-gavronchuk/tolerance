import { execFile } from 'node:child_process';
import { realpath } from 'node:fs/promises';
import { promisify } from 'node:util';

const run = promisify(execFile);

/** Runs system git in `cwd` and returns trimmed stdout. Output is in English and stable for parsing. */
export async function git(cwd: string, args: string[]): Promise<string> {
  const { stdout } = await run('git', args, {
    cwd,
    maxBuffer: 64 * 1024 * 1024,
    env: { ...process.env, LC_ALL: 'C', GIT_TERMINAL_PROMPT: '0' },
  });
  return stdout.trimEnd();
}

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
