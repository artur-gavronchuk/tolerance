import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { git } from './exec';

export type DiffFileStatus = 'added' | 'modified' | 'deleted' | 'other';

export interface DiffFile {
  path: string;
  status: DiffFileStatus;
  /** Lines added and removed; null for binary files. */
  additions: number | null;
  deletions: number | null;
}

export interface TaskDiff {
  mergeBase: string;
  files: DiffFile[];
  /** Unified diff text; cut at `maxPatchChars`. */
  patch: string;
  truncated: boolean;
}

const STATUS: Record<string, DiffFileStatus> = { A: 'added', M: 'modified', D: 'deleted' };

/**
 * What a task changed, from `git merge-base <base> <head>` (SPEC §3.4).
 *
 * In a worktree (no `head`): committed work plus the working tree, untracked files included. The
 * working tree is staged into a throwaway index, so the worktree's own index is left alone.
 * With `head` (a branch whose worktree is gone): just the commits.
 */
export async function diffFromMergeBase(input: {
  cwd: string;
  base: string;
  head?: string;
  maxPatchChars?: number;
}): Promise<TaskDiff> {
  const max = input.maxPatchChars ?? 2_000_000;
  const mergeBase = await git(input.cwd, ['merge-base', input.base, input.head ?? 'HEAD']);
  const common = ['diff', '--no-color', '--no-ext-diff', '--no-renames'];

  const collect = async (range: string[], env: NodeJS.ProcessEnv = {}) => {
    const [names, numstat, patch] = await Promise.all([
      git(input.cwd, [...common, '--name-status', '-z', ...range], env),
      git(input.cwd, [...common, '--numstat', '-z', ...range], env),
      git(input.cwd, [...common, ...range], env),
    ]);
    return { files: parseFiles(names, numstat), patch };
  };

  let result: { files: DiffFile[]; patch: string };
  if (input.head) {
    result = await collect([mergeBase, input.head]);
  } else {
    const dir = await mkdtemp(join(tmpdir(), 'studio-index-'));
    const env = { GIT_INDEX_FILE: join(dir, 'index') };
    try {
      await git(input.cwd, ['read-tree', 'HEAD'], env);
      await git(input.cwd, ['add', '-A'], env);
      result = await collect(['--cached', mergeBase], env);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  }

  const truncated = result.patch.length > max;
  return {
    mergeBase,
    files: result.files,
    patch: truncated ? result.patch.slice(0, max) : result.patch,
    truncated,
  };
}

function parseFiles(nameStatus: string, numstat: string): DiffFile[] {
  const counts = new Map<string, { additions: number | null; deletions: number | null }>();
  for (const entry of numstat.split('\0')) {
    const match = /^(\S+)\t(\S+)\t(.*)$/s.exec(entry);
    if (!match) continue;
    const [, added, deleted, path] = match;
    if (!path) continue;
    counts.set(path, {
      additions: added === '-' ? null : Number(added),
      deletions: deleted === '-' ? null : Number(deleted),
    });
  }
  const parts = nameStatus.split('\0').filter(Boolean);
  const files: DiffFile[] = [];
  for (let i = 0; i + 1 < parts.length; i += 2) {
    const code = parts[i] ?? '';
    const path = parts[i + 1] ?? '';
    files.push({
      path,
      status: STATUS[code[0] ?? ''] ?? 'other',
      ...(counts.get(path) ?? { additions: null, deletions: null }),
    });
  }
  return files.sort((a, b) => (a.path < b.path ? -1 : a.path > b.path ? 1 : 0));
}
