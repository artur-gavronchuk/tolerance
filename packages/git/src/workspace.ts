import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { git } from './exec';
import { branchExists, resolveCommit, slugify } from './worktree';

/**
 * Where a daily-mode workspace lives and which branch it is on:
 * `<root>/<project>/ws/<slug>` on `studio/ws/<slug>`.
 */
export function workspaceLayout(input: { root: string; projectName: string; slug: string }): {
  path: string;
  branch: string;
} {
  return {
    path: join(input.root, slugify(input.projectName) || 'project', 'ws', input.slug),
    branch: `studio/ws/${input.slug}`,
  };
}

/**
 * A slug for a new workspace that no branch, folder or other workspace uses yet: `name`, then
 * `name-2`, `name-3`… An old branch with the same name is never picked up silently.
 * Returns null when the name has nothing usable for a slug (e.g. only punctuation).
 */
export async function freeWorkspaceSlug(input: {
  repo: string;
  root: string;
  projectName: string;
  name: string;
  taken: ReadonlySet<string>;
}): Promise<string | null> {
  const base = slugify(input.name);
  if (!base) return null;
  for (let n = 1; n < 100; n++) {
    const slug = n === 1 ? base : `${base}-${n}`;
    if (input.taken.has(slug)) continue;
    const layout = workspaceLayout({ root: input.root, projectName: input.projectName, slug });
    if (existsSync(layout.path)) continue;
    if (await branchExists(input.repo, layout.branch)) continue;
    return slug;
  }
  return null;
}

export interface FreshBase {
  /** What the branch starts from, e.g. "origin/main" or, without a remote, "main". */
  ref: string;
  sha: string;
  /** Why `origin/<branch>` could not be refreshed; the workspace then starts from what is local. */
  fetchError: string | null;
}

/**
 * The newest known commit of the project's main branch. Fetches only that one branch into its
 * remote-tracking ref (the idea of emdash `create-worktree.ts`, Apache-2.0), so the user's own
 * branches are never touched. Without a remote, or when the fetch fails, falls back to the local
 * branch, then to HEAD.
 */
export async function freshBase(repo: string, branch: string, timeoutMs = 20_000): Promise<FreshBase> {
  let fetchError: string | null = null;
  const hasOrigin = await git(repo, ['remote', 'get-url', 'origin']).then(
    () => true,
    () => false,
  );
  if (hasOrigin) {
    try {
      await git(
        repo,
        [
          'fetch',
          '--quiet',
          '--no-tags',
          '--no-write-fetch-head',
          'origin',
          `+refs/heads/${branch}:refs/remotes/origin/${branch}`,
        ],
        {},
        { timeoutMs },
      );
    } catch (error) {
      fetchError = gitErrorText(error);
    }
    const remote = await resolveCommit(repo, `origin/${branch}`).catch(() => null);
    if (remote) return { ref: `origin/${branch}`, sha: remote, fetchError };
  }
  const local = await resolveCommit(repo, branch).catch(() => null);
  if (local) return { ref: branch, sha: local, fetchError };
  return { ref: 'HEAD', sha: await resolveCommit(repo, 'HEAD'), fetchError };
}

export interface WorktreeChanges {
  /** Paths with uncommitted changes, untracked files included. They are lost when the worktree goes. */
  uncommitted: string[];
  /** Commits made in the workspace (after `base`) that no remote branch has. They stay on the branch. */
  unpushed: number;
}

/** What deleting a worktree would affect; shown to the user before they confirm. */
export async function worktreeChanges(path: string, base: string): Promise<WorktreeChanges> {
  const status = await git(path, ['status', '--porcelain=v1', '--untracked-files=all', '-z']);
  const uncommitted: string[] = [];
  const entries = status.split('\0');
  for (let i = 0; i < entries.length; i++) {
    const entry = entries[i] ?? '';
    if (entry.length < 4) continue;
    uncommitted.push(entry.slice(3));
    // A rename or copy is followed by its source path.
    if (entry[0] === 'R' || entry[0] === 'C') i++;
  }
  const unpushed = Number(await git(path, ['rev-list', '--count', 'HEAD', '--not', '--remotes', base]));
  return { uncommitted, unpushed: Number.isFinite(unpushed) ? unpushed : 0 };
}

/** The useful part of a failed git command: its stderr, else the error message. */
export function gitErrorText(error: unknown): string {
  const stderr = (error as { stderr?: unknown }).stderr;
  const text = typeof stderr === 'string' && stderr.trim() ? stderr : String(error);
  return text.trim().split('\n').slice(-3).join('\n');
}
