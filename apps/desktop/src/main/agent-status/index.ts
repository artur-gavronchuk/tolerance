import { rmSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import type { StudioEvent } from '@studio/schema';
import type { AgentHooks } from '../terminals/terminals';
import { HookServer, writeClaudeHookSettings } from './hook-server';
import { WorkspaceStatuses } from './workspace-statuses';

/** `~/.studio`; `STUDIO_HOME` moves it, so tests never write to the user's folder. */
export function studioHome(): string {
  return process.env.STUDIO_HOME || join(homedir(), '.studio');
}

/**
 * Agent statuses (D3): the hook server, the `--settings` file for Claude and the per-workspace
 * statuses, joined into what terminals need. If the server cannot start, agents run without hooks:
 * their status then comes from output, silence and exit only.
 */
export async function startAgentStatus(deps: {
  notify: (event: StudioEvent) => void;
  setBadge: (count: number) => void;
  onSession: (terminalId: string, sessionId: string) => void;
}): Promise<{ statuses: WorkspaceStatuses; hooks: AgentHooks; stop: () => void }> {
  const statuses = new WorkspaceStatuses(deps);
  const server = new HookServer((terminalId, payload) => statuses.hook(terminalId, payload));
  let settingsPath: string | null = null;
  try {
    const port = await server.start();
    // One file per running Studio (the port differs), so two instances never redirect each other's hooks.
    settingsPath = writeClaudeHookSettings(studioHome(), port);
  } catch (error) {
    console.error('Agent hooks are off:', error);
  }
  const hooks: AgentHooks = {
    settingsPath,
    tokenFor: (id) => server.tokenFor(id),
    revoke: (id) => server.revoke(id),
    track: (id, owner, opts) => statuses.track(id, owner, opts),
    untrack: (id) => statuses.untrack(id),
  };
  const stop = () => {
    server.close();
    statuses.statuses.dispose();
    if (settingsPath) rmSync(settingsPath, { force: true });
  };
  return { statuses, hooks, stop };
}
