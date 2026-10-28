import { execFile } from 'node:child_process';
import { git } from './exec';
import { gitErrorText } from './workspace';

export interface BranchState {
  branch: string | null;
  /** Files with uncommitted changes, untracked included. */
  uncommitted: number;
  /** Commits on the branch since the merge-base with `base`. */
  commits: number;
  /** `origin/<branch>` when the branch was pushed with tracking, else null. */
  upstream: string | null;
  /** Commits not on the upstream yet; null without an upstream. */
  unpushed: number | null;
}

/** Where a workspace branch stands: what is uncommitted, committed and not pushed. */
export async function branchState(path: string, base: string): Promise<BranchState> {
  const [branch, status, commits, upstream] = await Promise.all([
    git(path, ['symbolic-ref', '--quiet', '--short', 'HEAD']).catch(() => null),
    git(path, ['status', '--porcelain=v1', '--untracked-files=all', '-z']),
    git(path, ['rev-list', '--count', `${base}..HEAD`]).catch(() => '0'),
    git(path, ['rev-parse', '--abbrev-ref', '--symbolic-full-name', '@{u}']).catch(() => null),
  ]);
  let entries = 0;
  const parts = status.split('\0');
  for (let i = 0; i < parts.length; i++) {
    const entry = parts[i] ?? '';
    if (entry.length < 4) continue;
    entries++;
    if (entry[0] === 'R' || entry[0] === 'C') i++;
  }
  const unpushed = upstream ? Number(await git(path, ['rev-list', '--count', `${upstream}..HEAD`])) : null;
  return { branch, uncommitted: entries, commits: Number(commits) || 0, upstream, unpushed };
}

/** Why a commit or push could not happen; `detail` is git's own words. */
export class GitActionError extends Error {
  constructor(
    readonly code:
      | 'nothing_to_commit'
      | 'empty_message'
      | 'hook_failed'
      | 'not_studio_branch'
      | 'push_failed',
    readonly detail = '',
  ) {
    super(detail ? `${code}: ${detail}` : code);
  }
}

/**
 * Commits everything in the worktree, untracked files included, with the developer's message.
 * Unlike the commit Studio makes for an agent, the repository's hooks run: it is the developer's
 * own commit.
 */
export async function commitWorkspace(path: string, message: string): Promise<string> {
  if (!message.trim()) throw new GitActionError('empty_message');
  await git(path, ['add', '-A']);
  if (!(await git(path, ['diff', '--cached', '--name-only']))) throw new GitActionError('nothing_to_commit');
  try {
    // The message is one argv entry: no shell, no quoting issues.
    await git(path, ['commit', '-q', '-m', message.trim()]);
  } catch (error) {
    throw new GitActionError('hook_failed', gitErrorText(error));
  }
  return git(path, ['rev-parse', 'HEAD']);
}

/**
 * Pushes the workspace branch to `origin` under the same name, setting the upstream the first time
 * (the emdash `git-checkout.ts` flow, rewritten). Only `studio/*` branches: Studio never pushes the
 * user's own branches.
 */
export async function pushBranch(path: string): Promise<void> {
  const branch = await git(path, ['symbolic-ref', '--quiet', '--short', 'HEAD']).catch(() => '');
  if (!branch.startsWith('studio/')) throw new GitActionError('not_studio_branch', branch);
  const upstream = await git(path, ['rev-parse', '--abbrev-ref', '@{u}']).catch(() => null);
  const args = upstream
    ? ['push', '--quiet', 'origin', `HEAD:refs/heads/${branch}`]
    : ['push', '--quiet', '--set-upstream', 'origin', `HEAD:refs/heads/${branch}`];
  try {
    await git(path, args, {}, { timeoutMs: 120_000 });
  } catch (error) {
    throw new GitActionError('push_failed', gitErrorText(error));
  }
}

/** `https://github.com/owner/repo` for an `origin` on GitHub (ssh or https), else null. */
export async function githubRepoUrl(path: string): Promise<string | null> {
  const url = await git(path, ['remote', 'get-url', 'origin']).catch(() => '');
  const match = /github\.com[:/]([^/]+)\/(.+?)(?:\.git)?\/?$/.exec(url);
  return match ? `https://github.com/${match[1]}/${match[2]}` : null;
}

// ── GitHub CLI ──────────────────────────────────────────────────────────────

/** Runs `gh`; never prompts. Rejects with the `gh` error, or `ENOENT` when it is not installed. */
function gh(cwd: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile(
      'gh',
      args,
      { cwd, timeout: 60_000, env: { ...process.env, GH_PROMPT_DISABLED: '1', NO_COLOR: '1' } },
      (error, stdout, stderr) => {
        if (error) reject(Object.assign(error, { stderr }));
        else resolve(stdout.trim());
      },
    );
  });
}

export type GhState = 'ok' | 'missing' | 'logged_out';

export class GhError extends Error {
  constructor(
    readonly code: GhState | 'failed',
    readonly detail = '',
  ) {
    super(detail ? `${code}: ${detail}` : code);
  }
}

function asGhError(error: unknown): GhError {
  const e = error as { code?: unknown; stderr?: unknown };
  if (e.code === 'ENOENT') return new GhError('missing');
  const stderr = String(e.stderr ?? '');
  // gh exits with 4 when it needs `gh auth login` (as vibe-kanban `git-host/github/cli.rs` treats it).
  if (e.code === 4 || /gh auth login|not logged in|authentication/i.test(stderr))
    return new GhError('logged_out');
  return new GhError('failed', stderr.trim().split('\n').slice(-3).join('\n') || String(error));
}

/** The open pull request of the branch, if there is one. */
export async function findPullRequest(path: string, branch: string): Promise<string | null> {
  try {
    const out = await gh(path, [
      'pr',
      'list',
      '--head',
      branch,
      '--state',
      'open',
      '--json',
      'url',
      '--limit',
      '1',
    ]);
    const list = JSON.parse(out || '[]') as { url: string }[];
    return list[0]?.url ?? null;
  } catch (error) {
    throw asGhError(error);
  }
}

/** Opens a pull request of `branch` into `base`, titled and described from its commits. Returns its URL. */
export async function createPullRequest(
  path: string,
  input: { branch: string; base: string },
): Promise<string> {
  try {
    const out = await gh(path, ['pr', 'create', '--head', input.branch, '--base', input.base, '--fill']);
    const url = /https:\/\/\S+\/pull\/\d+/.exec(out)?.[0];
    if (!url) throw new GhError('failed', out);
    return url;
  } catch (error) {
    throw error instanceof GhError ? error : asGhError(error);
  }
}
