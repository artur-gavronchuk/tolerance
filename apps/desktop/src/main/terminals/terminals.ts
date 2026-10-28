import { randomUUID } from 'node:crypto';
import { existsSync } from 'node:fs';
import { basename } from 'node:path';
import type { Db } from '@studio/db';
import type { Project, StudioEvent, Terminal, TerminalProcess, Workspace } from '@studio/schema';
import { HOOK_TOKEN_ENV } from '../agent-status/hook-server';
import type { PtyHost, PtySnapshot } from './pty-host';

export const DEFAULT_AGENT = 'claude';

/** The user's shell; `/bin/sh` when `$SHELL` is unset or missing. */
export function userShell(env: NodeJS.ProcessEnv = process.env): string {
  return env.SHELL && existsSync(env.SHELL) ? env.SHELL : '/bin/sh';
}

/** Interactive login shell flags; plain sh and dash only take `-i` (as in emdash `shell-ids.ts`). */
function shellFlags(shell: string): string[] {
  return ['sh', 'dash'].includes(basename(shell)) ? ['-i'] : ['-i', '-l'];
}

/** Whether the agent command starts Claude Code, which takes `--session-id`. */
export function isClaude(command: string): boolean {
  const first = command.trim().split(/\s+/)[0] ?? '';
  return basename(first) === 'claude';
}

/**
 * What a terminal tab runs. The agent goes through the login shell, so PATH, nvm and aliases are
 * the user's own; `exec` leaves no shell behind it, and the tab ends when the agent does.
 */
export function terminalCommand(input: {
  kind: Terminal['kind'];
  agentCommand: string | null;
  sessionId: string | null;
  /** Continue `sessionId` (`--resume`) instead of starting it (`--session-id`). */
  resume?: boolean;
  /** `claude --settings <file>` with Studio's status hooks; null when the hook server is down. */
  hookSettings?: string | null;
  shell: string;
}): { file: string; args: string[] } {
  const flags = shellFlags(input.shell);
  if (input.kind === 'shell') return { file: input.shell, args: flags };
  const agent = input.agentCommand?.trim() || DEFAULT_AGENT;
  const extra: string[] = [];
  if (isClaude(agent)) {
    if (input.hookSettings) extra.push('--settings', shellQuote(input.hookSettings));
    if (input.sessionId) extra.push(input.resume ? '--resume' : '--session-id', input.sessionId);
  }
  const line = ['exec', agent, ...extra].join(' ');
  return { file: input.shell, args: [...flags, '-c', line] };
}

/** Variables that belong to Studio's own process or to a Claude Code session Studio was started from. */
const DROPPED = /^(ELECTRON_|CLAUDECODE$|CLAUDE_CODE_ENTRYPOINT$|STUDIO_USER_DATA$|STUDIO_WORKTREES_ROOT$)/;

/** The environment of a workspace terminal (one place for it, as emdash `terminal-env.ts` advises). */
export function terminalEnv(
  base: NodeJS.ProcessEnv,
  ctx: { project: Project; workspace: Workspace },
): NodeJS.ProcessEnv {
  const env: NodeJS.ProcessEnv = {};
  for (const [key, value] of Object.entries(base)) {
    if (value !== undefined && !DROPPED.test(key)) env[key] = value;
  }
  return {
    ...env,
    TERM: 'xterm-256color',
    COLORTERM: 'truecolor',
    TERM_PROGRAM: 'Studio',
    LANG: env.LANG || 'en_US.UTF-8',
    STUDIO_ROOT_PATH: ctx.project.repoPath,
    STUDIO_WORKSPACE_PATH: ctx.workspace.path,
    STUDIO_WORKSPACE_NAME: ctx.workspace.name,
    STUDIO_BRANCH: ctx.workspace.branch,
  };
}

/** One argument for `sh -c`, safe whatever it contains. */
function shellQuote(value: string): string {
  return `'${value.replaceAll("'", `'\\''`)}'`;
}

export class TerminalStateError extends Error {}

/** A resumed agent that exits with an error sooner than this had nothing to resume. */
const RESUME_FALLBACK_MS = 5000;

interface StartRecord {
  ctx: { project: Project; workspace: Workspace; terminal: Terminal };
  size: { cols: number; rows: number };
  resume: boolean;
  at: number;
}

/** How an agent tab reports its status (D3): the hook settings file and a token per start. */
export interface AgentHooks {
  settingsPath: string | null;
  tokenFor(terminalId: string): string;
  revoke(terminalId: string): void;
  track(
    terminalId: string,
    owner: { workspaceId: string; projectId: string; title: string },
    opts: { hooks: boolean },
  ): void;
  untrack(terminalId: string): void;
}

/**
 * Terminal tabs of workspaces: rows in SQLite, processes in {@link PtyHost}. A new workspace gets
 * the agent tab and a shell tab.
 */
export class Terminals {
  constructor(
    private readonly deps: {
      db: Db;
      host: PtyHost;
      notify: (event: StudioEvent) => void;
      hooks?: AgentHooks;
    },
  ) {}

  async list(workspaceId: string): Promise<{ terminal: Terminal; process: TerminalProcess }[]> {
    const rows = await this.deps.db.listTerminals(workspaceId);
    return rows.map((terminal) => ({ terminal, process: this.deps.host.process(terminal.id) }));
  }

  /** The default tabs, the first time a workspace is opened. */
  async ensureDefaults(workspaceId: string): Promise<void> {
    if ((await this.deps.db.listTerminals(workspaceId)).length > 0) return;
    await this.deps.db.addTerminal({ workspaceId, kind: 'agent' });
    await this.deps.db.addTerminal({ workspaceId, kind: 'shell' });
  }

  async addShell(workspaceId: string): Promise<Terminal> {
    return this.deps.db.addTerminal({ workspaceId, kind: 'shell' });
  }

  /**
   * Starts the tab's program at the size the UI measured. An agent tab either continues its stored
   * Claude session (`resume`, D4) or starts a new one with a fresh id, stored before the process
   * starts: an id cannot be reused ("already in use").
   */
  async start(
    ctx: { project: Project; workspace: Workspace; terminal: Terminal },
    size: { cols: number; rows: number },
    opts: { resume?: boolean } = {},
  ): Promise<void> {
    const { db, host } = this.deps;
    const { project, workspace, terminal } = ctx;
    if (host.process(terminal.id).state === 'running') return;
    if (!existsSync(workspace.path)) throw new TerminalStateError('Workspace folder is missing');

    const claude = terminal.kind === 'agent' && isClaude(project.agentCommand ?? DEFAULT_AGENT);
    const resume = claude && Boolean(opts.resume && terminal.sessionId);
    let sessionId: string | null = resume ? terminal.sessionId : null;
    if (claude && !resume) {
      sessionId = randomUUID();
      await db.setTerminalSession(terminal.id, sessionId);
    }
    this.starts.set(terminal.id, { ctx, size, resume, at: Date.now() });
    const env = terminalEnv(process.env, { project, workspace });
    const hooks = this.deps.hooks;
    const hookSettings = claude ? (hooks?.settingsPath ?? null) : null;
    if (hooks && hookSettings) env[HOOK_TOKEN_ENV] = hooks.tokenFor(terminal.id);
    const command = terminalCommand({
      kind: terminal.kind,
      agentCommand: project.agentCommand,
      sessionId,
      resume,
      hookSettings,
      shell: userShell(),
    });
    host.start(terminal.id, { ...command, cwd: workspace.path, env, ...size });
    if (terminal.kind === 'agent') {
      hooks?.track(
        terminal.id,
        { workspaceId: workspace.id, projectId: project.id, title: `${project.name} / ${workspace.name}` },
        { hooks: Boolean(hookSettings) },
      );
    }
    this.deps.notify({ type: 'terminal', workspaceId: workspace.id, terminalId: terminal.id });
  }

  /**
   * A tab's program ended. A resume that dies within seconds with an error means Claude has no such
   * session (e.g. it was never written: nothing was typed in it): start a new one instead, as emdash
   * `tui-agents/runtime.ts` does. Returns whether it did.
   */
  async exited(terminalId: string, exitCode: number | null): Promise<boolean> {
    const last = this.starts.get(terminalId);
    this.starts.delete(terminalId);
    if (!last?.resume || exitCode === 0 || Date.now() - last.at > RESUME_FALLBACK_MS) return false;
    const terminal = await this.deps.db.getTerminal(terminalId);
    if (!terminal) return false;
    await this.start({ ...last.ctx, terminal }, last.size);
    return true;
  }

  snapshot(terminalId: string): PtySnapshot {
    return this.deps.host.snapshot(terminalId);
  }

  /** Closes a tab: stops its program and removes it. */
  async close(terminal: Terminal): Promise<void> {
    this.forget(terminal.id);
    await this.deps.db.removeTerminal(terminal.id);
  }

  /** Stops every program of a workspace, e.g. before its folder is removed. */
  async stopWorkspace(workspaceId: string): Promise<void> {
    for (const terminal of await this.deps.db.listTerminals(workspaceId)) this.forget(terminal.id);
  }

  private readonly starts = new Map<string, StartRecord>();

  private forget(terminalId: string): void {
    this.starts.delete(terminalId);
    this.deps.host.dispose(terminalId);
    this.deps.hooks?.revoke(terminalId);
    this.deps.hooks?.untrack(terminalId);
  }
}
