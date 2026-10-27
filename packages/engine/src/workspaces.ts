import { type ChildProcess, spawn } from 'node:child_process';
import { existsSync } from 'node:fs';
import type { Db } from '@studio/db';
import {
  createWorktree,
  defaultWorktreesRoot,
  freeWorkspaceSlug,
  freshBase,
  removeWorktree,
  type WorktreeChanges,
  workspaceLayout,
  worktreeChanges,
} from '@studio/git';
import type { Project, StudioEvent, Workspace } from '@studio/schema';

export interface WorkspacesDeps {
  db: Db;
  notify: (event: StudioEvent) => void;
  /** `~/.studio/worktrees` unless set (tests). */
  worktreesRoot?: string;
  /** How long to wait for `git fetch` of the main branch before using what is local. */
  fetchTimeoutMs?: number;
}

/** A request about a workspace that cannot be done; `code` is an i18n key for the UI. */
export class WorkspaceError extends Error {
  constructor(readonly code: 'bad_name' | 'setup_running' | 'no_setup') {
    super(code);
  }
}

/** Setup output kept per workspace, in memory while it runs and in the database after. */
const LOG_LIMIT = 64 * 1024;
const NOTIFY_EVERY_MS = 300;

interface SetupProcess {
  child: ChildProcess;
  raw: string;
  timer: ReturnType<typeof setTimeout> | null;
}

/**
 * Daily mode: a workspace is a `studio/ws/<slug>` branch in its own worktree, where the developer
 * works by hand. After creation the project's `setupCommand` runs there and its output is shown.
 */
export class Workspaces {
  private readonly setups = new Map<string, SetupProcess>();

  constructor(private readonly deps: WorkspacesDeps) {}

  get root(): string {
    return this.deps.worktreesRoot ?? defaultWorktreesRoot();
  }

  /**
   * Branches from the freshest `origin/<main>` (local main without a remote or network), adds the
   * worktree and starts the setup command in the background.
   */
  async create(project: Project, name: string): Promise<{ workspace: Workspace; fetchError: string | null }> {
    const { db, notify } = this.deps;
    const taken = new Set((await db.listWorkspaces(project.id)).map((w) => w.slug));
    const slug = await freeWorkspaceSlug({
      repo: project.repoPath,
      root: this.root,
      projectName: project.name,
      name,
      taken,
    });
    if (!slug) throw new WorkspaceError('bad_name');

    const { path, branch } = workspaceLayout({ root: this.root, projectName: project.name, slug });
    const base = await freshBase(project.repoPath, project.defaultBranch, this.deps.fetchTimeoutMs);
    await createWorktree({ repo: project.repoPath, path, branch, base: base.sha });

    const setup = project.setupCommand?.trim();
    const workspace = await db.addWorkspace({
      projectId: project.id,
      name: name.trim(),
      slug,
      branch,
      path,
      baseRef: base.ref,
      baseSha: base.sha,
      setupStatus: setup ? 'running' : 'none',
    });
    if (setup) this.startSetup(project, workspace, setup);
    notify({ type: 'workspace', projectId: project.id, workspaceId: workspace.id });
    return { workspace, fetchError: base.fetchError };
  }

  /** Runs the setup command again, e.g. after it failed or was cut off by a restart. */
  async retrySetup(project: Project, workspace: Workspace): Promise<void> {
    const setup = project.setupCommand?.trim();
    if (!setup) throw new WorkspaceError('no_setup');
    if (this.setups.has(workspace.id)) throw new WorkspaceError('setup_running');
    await this.deps.db.setWorkspaceSetup(workspace.id, { status: 'running', exitCode: null, log: '' });
    this.startSetup(project, workspace, setup);
    this.deps.notify({ type: 'workspace', projectId: project.id, workspaceId: workspace.id });
  }

  /** Setup output so far: live while the command runs, from the database after. */
  setupLog(workspace: Workspace): string {
    const live = this.setups.get(workspace.id);
    return live ? cleanLog(live.raw) : workspace.setupLog;
  }

  /** What deleting would lose (uncommitted files) and keep (commits on the branch). */
  async changes(workspace: Workspace): Promise<WorktreeChanges & { missing: boolean }> {
    if (!existsSync(workspace.path)) return { uncommitted: [], unpushed: 0, missing: true };
    return { ...(await worktreeChanges(workspace.path, workspace.baseSha)), missing: false };
  }

  /** Stops setup, removes the worktree folder (only inside the Studio root) and forgets the workspace. The branch stays. */
  async remove(project: Project, workspace: Workspace): Promise<void> {
    await this.stopSetup(workspace.id);
    await removeWorktree({ repo: project.repoPath, root: this.root, path: workspace.path });
    await this.deps.db.removeWorkspace(workspace.id);
    this.deps.notify({ type: 'workspace', projectId: project.id, workspaceId: workspace.id });
  }

  /** On app quit: setup commands do not outlive Studio. */
  killAll(): void {
    for (const setup of this.setups.values()) killGroup(setup.child, 'SIGKILL');
  }

  private startSetup(project: Project, workspace: Workspace, command: string): void {
    const shell = process.env.SHELL && existsSync(process.env.SHELL) ? process.env.SHELL : '/bin/sh';
    const env: NodeJS.ProcessEnv = {
      ...process.env,
      STUDIO_ROOT_PATH: project.repoPath,
      STUDIO_WORKSPACE_PATH: workspace.path,
      STUDIO_WORKSPACE_NAME: workspace.name,
      STUDIO_BRANCH: workspace.branch,
    };
    // Electron's own switches must not leak into the user's tools.
    delete env.ELECTRON_RUN_AS_NODE;

    // Own process group, so stopping it also stops what it started (pnpm, node, …).
    const child = spawn(shell, ['-lc', command], {
      cwd: workspace.path,
      env,
      detached: true,
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    const setup: SetupProcess = { child, raw: '', timer: null };
    this.setups.set(workspace.id, setup);

    const changed = () => {
      setup.timer ??= setTimeout(() => {
        setup.timer = null;
        this.deps.notify({ type: 'workspace', projectId: project.id, workspaceId: workspace.id });
      }, NOTIFY_EVERY_MS);
    };
    const append = (chunk: Buffer) => {
      setup.raw += chunk.toString('utf8');
      if (setup.raw.length > LOG_LIMIT * 2) setup.raw = setup.raw.slice(-LOG_LIMIT);
      changed();
    };
    child.stdout?.on('data', append);
    child.stderr?.on('data', append);

    let finished = false;
    const finish = async (exitCode: number | null, error?: Error) => {
      if (finished) return;
      finished = true;
      if (setup.timer) clearTimeout(setup.timer);
      if (error) setup.raw += `\n${error.message}\n`;
      this.setups.delete(workspace.id);
      // The workspace may be gone already (deleted while setup ran).
      if (await this.deps.db.getWorkspace(workspace.id)) {
        await this.deps.db.setWorkspaceSetup(workspace.id, {
          status: exitCode === 0 ? 'done' : 'failed',
          exitCode,
          log: cleanLog(setup.raw),
        });
      }
      this.deps.notify({ type: 'workspace', projectId: project.id, workspaceId: workspace.id });
    };
    child.on('error', (error) => void finish(null, error));
    child.on('close', (code) => void finish(code));
  }

  private async stopSetup(workspaceId: string): Promise<void> {
    const setup = this.setups.get(workspaceId);
    if (!setup) return;
    const closed = new Promise<void>((resolve) => setup.child.once('close', () => resolve()));
    killGroup(setup.child, 'SIGTERM');
    const timer = setTimeout(() => killGroup(setup.child, 'SIGKILL'), 2000);
    await closed;
    clearTimeout(timer);
  }
}

function killGroup(child: ChildProcess, signal: NodeJS.Signals): void {
  if (child.pid === undefined || child.exitCode !== null) return;
  try {
    process.kill(-child.pid, signal);
  } catch {
    child.kill(signal);
  }
}

// biome-ignore lint/suspicious/noControlCharactersInRegex: matching terminal escape sequences is the point.
const ANSI = /\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[@-Z\\-_]/g;

/**
 * Setup output as plain text: colors dropped, and a line redrawn with `\r` (progress bars) shows
 * only its last state. Keeps the last {@link LOG_LIMIT} characters.
 */
export function cleanLog(raw: string): string {
  const text = raw
    .replace(ANSI, '')
    .replace(/\r\n/g, '\n')
    .split('\n')
    .map((line) => line.slice(line.lastIndexOf('\r') + 1))
    .join('\n');
  return text.length > LOG_LIMIT ? text.slice(-LOG_LIMIT) : text;
}
