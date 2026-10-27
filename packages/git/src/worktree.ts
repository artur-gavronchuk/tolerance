import { existsSync } from 'node:fs';
import { mkdir, realpath, rm } from 'node:fs/promises';
import { homedir } from 'node:os';
import { isAbsolute, join, relative } from 'node:path';
import { git } from './exec';

/**
 * Where task worktrees live (SPEC §3.4): `~/.studio/worktrees`. `STUDIO_WORKTREES_ROOT` moves it,
 * so e2e tests never touch the user's folder.
 */
export function defaultWorktreesRoot(): string {
  return process.env.STUDIO_WORKTREES_ROOT || join(homedir(), '.studio', 'worktrees');
}

const CYRILLIC: Record<string, string> = {
  а: 'a',
  б: 'b',
  в: 'v',
  г: 'g',
  д: 'd',
  е: 'e',
  ё: 'e',
  ж: 'zh',
  з: 'z',
  и: 'i',
  й: 'y',
  к: 'k',
  л: 'l',
  м: 'm',
  н: 'n',
  о: 'o',
  п: 'p',
  р: 'r',
  с: 's',
  т: 't',
  у: 'u',
  ф: 'f',
  х: 'h',
  ц: 'ts',
  ч: 'ch',
  ш: 'sh',
  щ: 'sch',
  ъ: '',
  ы: 'y',
  ь: '',
  э: 'e',
  ю: 'yu',
  я: 'ya',
};

/** A short ASCII name for paths and branches: Russian is transliterated, everything else becomes "-". */
export function slugify(text: string, maxLength = 40): string {
  const ascii = [...text.toLowerCase()].map((ch) => CYRILLIC[ch] ?? ch).join('');
  const slug = ascii
    .replace(/[^a-z0-9]+/g, '-')
    .slice(0, maxLength)
    .replace(/^-+|-+$/g, '');
  return slug;
}

/** A task key such as "T1" made safe for a path segment and a ref name. */
function safeKey(key: string): string {
  return key.replace(/[^A-Za-z0-9_]+/g, '-').replace(/^-+|-+$/g, '') || 'task';
}

/** Where a task's worktree goes and which branch it is on (SPEC §3.4). */
export function taskWorktreeLayout(input: {
  root: string;
  projectName: string;
  planId: string;
  taskKey: string;
  taskTitle: string;
}): { path: string; branch: string } {
  const key = safeKey(input.taskKey);
  const titleSlug = slugify(input.taskTitle);
  return {
    path: join(input.root, slugify(input.projectName) || 'project', input.planId, key),
    branch: `studio/${input.planId.slice(0, 8)}/${titleSlug ? `${key}-${titleSlug}` : key}`,
  };
}

async function branchExists(repo: string, branch: string): Promise<boolean> {
  return git(repo, ['show-ref', '--verify', '--quiet', `refs/heads/${branch}`]).then(
    () => true,
    () => false,
  );
}

/** The commit a branch name (or any ref) points to; fails when it does not exist. */
export async function resolveCommit(repo: string, ref: string): Promise<string> {
  return git(repo, ['rev-parse', '--verify', '--quiet', `${ref}^{commit}`]);
}

/**
 * Creates the task worktree at `path` on `branch`, branching from `base`. If the branch is already
 * there (the worktree was removed earlier), it is checked out again with its commits, never reset.
 * If the worktree already exists, it is reused as is.
 */
export async function createWorktree(input: {
  repo: string;
  path: string;
  branch: string;
  base: string;
}): Promise<{ path: string; created: boolean }> {
  if (existsSync(join(input.path, '.git'))) return { path: input.path, created: false };
  await mkdir(join(input.path, '..'), { recursive: true });
  // A worktree directory deleted by hand is still registered until pruned.
  await git(input.repo, ['worktree', 'prune']);
  if (await branchExists(input.repo, input.branch)) {
    await git(input.repo, ['worktree', 'add', input.path, input.branch]);
  } else {
    const base = await resolveCommit(input.repo, input.base);
    await git(input.repo, ['worktree', 'add', '-b', input.branch, input.path, base]);
  }
  return { path: input.path, created: true };
}

export class UnsafeWorktreePathError extends Error {
  constructor(readonly path: string) {
    super(`Refusing to remove ${path}: it is not inside the Studio worktrees folder`);
  }
}

/** Whether `path` is strictly inside `root` once both are resolved through symlinks. */
export async function isInside(root: string, path: string): Promise<boolean> {
  const [realRoot, realPath] = await Promise.all([realpath(root), realpath(path)]);
  const rel = relative(realRoot, realPath);
  return rel !== '' && !rel.startsWith('..') && !isAbsolute(rel);
}

/**
 * Removes a task worktree: `git worktree remove --force` and `prune` (SPEC §3.4). Anything outside
 * `root` after realpath is refused. The branch and its commits stay.
 */
export async function removeWorktree(input: { repo: string; root: string; path: string }): Promise<void> {
  if (existsSync(input.path)) {
    if (!(await isInside(input.root, input.path))) throw new UnsafeWorktreePathError(input.path);
    const real = await realpath(input.path);
    try {
      await git(input.repo, ['worktree', 'remove', '--force', real]);
    } catch (error) {
      // Not a registered worktree any more (e.g. its .git file is gone): the folder is still ours to clear.
      if (!existsSync(real)) throw error;
      await rm(real, { recursive: true, force: true });
    }
  }
  await git(input.repo, ['worktree', 'prune']);
}

/**
 * Commits everything in the worktree, untracked files included, when there is something to commit.
 * Hooks are skipped: the worktree has no installed dependencies, and this is Studio's own branch.
 * Returns the head after it.
 */
export async function commitAll(
  worktree: string,
  message: string,
): Promise<{ sha: string; committed: boolean }> {
  await git(worktree, ['add', '-A']);
  const staged = await git(worktree, ['diff', '--cached', '--name-only']);
  let committed = false;
  if (staged) {
    const identity = (await git(worktree, ['config', 'user.email']).catch(() => ''))
      ? []
      : ['-c', 'user.name=Studio', '-c', 'user.email=studio@localhost'];
    await git(worktree, [...identity, 'commit', '--no-verify', '-q', '-m', message]);
    committed = true;
  }
  return { sha: await git(worktree, ['rev-parse', 'HEAD']), committed };
}
