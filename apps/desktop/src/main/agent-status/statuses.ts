import type { AgentStatus } from '@studio/schema';
import type { HookPayload } from './hook-server';

/** SPEC §3.3: no events for this long while working means the agent is stuck. */
export const SILENCE_MS = 10 * 60 * 1000;

/** Notification types that mean Claude waits for an answer (a permission, a question). */
const ASKS = new Set(['permission_prompt', 'elicitation_dialog', 'agent_needs_input']);

export interface StatusChange {
  terminalId: string;
  status: AgentStatus;
  /** Why it changed, for deciding whether to notify: a hook, process output, silence, exit. */
  cause: 'hook' | 'output' | 'silence' | 'exit' | 'start';
}

interface Entry {
  status: AgentStatus;
  /** Claude reports through hooks; other agents only through their output and exit. */
  hooks: boolean;
  timer: ReturnType<typeof setTimeout> | null;
}

/**
 * The status of each agent terminal. With hooks: UserPromptSubmit and PostToolUse → working,
 * PermissionRequest and a Notification asking for input → needs_input, Stop → idle. Without hooks:
 * output → working. Either way, working and silent for {@link SILENCE_MS} → stalled, exit → exited.
 * A status is reported only when it changes.
 */
export class AgentStatuses {
  private readonly entries = new Map<string, Entry>();

  constructor(
    private readonly onChange: (change: StatusChange) => void,
    private readonly silenceMs = SILENCE_MS,
  ) {}

  get(terminalId: string): AgentStatus | undefined {
    return this.entries.get(terminalId)?.status;
  }

  /** A freshly started agent waits for the first prompt. */
  started(terminalId: string, opts: { hooks: boolean }): void {
    this.stopTimer(terminalId);
    this.entries.set(terminalId, { status: 'idle', hooks: opts.hooks, timer: null });
    this.onChange({ terminalId, status: 'idle', cause: 'start' });
  }

  hook(terminalId: string, payload: HookPayload): void {
    const entry = this.entries.get(terminalId);
    if (!entry) return;
    const event = payload.hook_event_name;
    if (event === 'UserPromptSubmit' || event === 'PostToolUse') this.set(terminalId, 'working', 'hook');
    else if (event === 'PermissionRequest') this.set(terminalId, 'needs_input', 'hook');
    else if (event === 'Notification' && ASKS.has(payload.notification_type ?? '')) {
      this.set(terminalId, 'needs_input', 'hook');
    } else if (event === 'Stop') this.set(terminalId, 'idle', 'hook');
    // Notification `idle_prompt` comes a minute after Stop: already idle, nothing new to report.
  }

  /** The agent printed something: it is alive. */
  output(terminalId: string): void {
    const entry = this.entries.get(terminalId);
    if (!entry) return;
    if (!entry.hooks && entry.status !== 'working') this.set(terminalId, 'working', 'output');
    else if (entry.status === 'working') this.armSilence(terminalId);
    else if (entry.status === 'stalled') this.set(terminalId, 'working', 'output');
  }

  exited(terminalId: string): void {
    if (!this.entries.has(terminalId)) return;
    this.set(terminalId, 'exited', 'exit');
  }

  remove(terminalId: string): void {
    this.stopTimer(terminalId);
    this.entries.delete(terminalId);
  }

  dispose(): void {
    for (const id of this.entries.keys()) this.stopTimer(id);
  }

  private set(terminalId: string, status: AgentStatus, cause: StatusChange['cause']): void {
    const entry = this.entries.get(terminalId);
    if (!entry) return;
    if (status === 'working') this.armSilence(terminalId);
    else this.stopTimer(terminalId);
    if (entry.status === status) return;
    entry.status = status;
    this.onChange({ terminalId, status, cause });
  }

  private armSilence(terminalId: string): void {
    const entry = this.entries.get(terminalId);
    if (!entry) return;
    if (entry.timer) clearTimeout(entry.timer);
    entry.timer = setTimeout(() => {
      entry.timer = null;
      if (entry.status === 'working') this.set(terminalId, 'stalled', 'silence');
    }, this.silenceMs);
    entry.timer.unref?.();
  }

  private stopTimer(terminalId: string): void {
    const entry = this.entries.get(terminalId);
    if (entry?.timer) clearTimeout(entry.timer);
    if (entry) entry.timer = null;
  }
}
