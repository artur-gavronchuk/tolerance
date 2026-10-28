import type { AgentStatus, StudioEvent, WorkspaceStatus } from '@studio/schema';
import type { HookPayload } from './hook-server';
import { AgentStatuses, type StatusChange } from './statuses';

export interface Owner {
  workspaceId: string;
  projectId: string;
  /** "project / workspace", shown in notifications. */
  title: string;
}

/** Statuses the developer should come back for. */
const CALLS: ReadonlySet<AgentStatus> = new Set(['needs_input', 'idle', 'stalled']);

/**
 * Agent statuses per workspace, and what the developer has not seen yet. A question, a finished
 * turn or a stall marks the workspace for attention (and an OS notification) unless the developer
 * is looking at it right now; opening it clears the mark. The count of workspaces waiting goes to
 * the dock badge.
 */
export class WorkspaceStatuses {
  private readonly owners = new Map<string, Owner>(); // agent terminal → workspace
  private readonly agentOf = new Map<string, string>(); // workspace → its agent terminal
  private readonly attention = new Set<string>();
  private viewing: string | null = null;
  readonly statuses: AgentStatuses;

  constructor(
    private readonly deps: {
      notify: (event: StudioEvent) => void;
      setBadge: (count: number) => void;
      /** Claude starts a new session on /clear and /resume; D4 resumes the latest one. */
      onSession: (terminalId: string, sessionId: string) => void;
      silenceMs?: number;
    },
  ) {
    this.statuses = new AgentStatuses((change) => this.changed(change), deps.silenceMs);
  }

  /** An agent tab started; `hooks` when it reports through Claude Code hooks. */
  track(terminalId: string, owner: Owner, opts: { hooks: boolean }): void {
    this.owners.set(terminalId, owner);
    this.agentOf.set(owner.workspaceId, terminalId);
    this.statuses.started(terminalId, opts);
  }

  untrack(terminalId: string): void {
    const owner = this.owners.get(terminalId);
    this.statuses.remove(terminalId);
    this.owners.delete(terminalId);
    if (owner && this.agentOf.get(owner.workspaceId) === terminalId) {
      this.agentOf.delete(owner.workspaceId);
      this.attention.delete(owner.workspaceId);
      this.publish(owner, null);
    }
  }

  hook(terminalId: string, payload: HookPayload): void {
    if (payload.session_id) this.deps.onSession(terminalId, payload.session_id);
    this.statuses.hook(terminalId, payload);
  }

  /** What the developer looks at: the open workspace while the window has focus, else null. */
  view(workspaceId: string | null): void {
    this.viewing = workspaceId;
    if (workspaceId && this.attention.delete(workspaceId)) {
      const terminalId = this.agentOf.get(workspaceId);
      const owner = terminalId ? this.owners.get(terminalId) : undefined;
      if (owner) this.publish(owner, null);
    }
  }

  list(): WorkspaceStatus[] {
    const out: WorkspaceStatus[] = [];
    for (const [workspaceId, terminalId] of this.agentOf) {
      const owner = this.owners.get(terminalId);
      const status = this.statuses.get(terminalId);
      if (owner && status) {
        out.push({
          workspaceId,
          projectId: owner.projectId,
          status,
          attention: this.attention.has(workspaceId),
        });
      }
    }
    return out;
  }

  /** Workspaces waiting for the developer: asking now, or with something unseen. */
  waitingCount(): number {
    return this.list().filter((s) => s.status === 'needs_input' || s.attention).length;
  }

  private changed(change: StatusChange): void {
    const owner = this.owners.get(change.terminalId);
    if (!owner) return;
    let notify: 'needs_input' | 'idle' | 'stalled' | null = null;
    if (change.status === 'working' || change.status === 'exited') {
      // Working again means the developer answered; an exit is their own doing or visible in the tab.
      this.attention.delete(owner.workspaceId);
    } else if (CALLS.has(change.status) && change.cause !== 'start' && this.viewing !== owner.workspaceId) {
      this.attention.add(owner.workspaceId);
      notify = change.status as 'needs_input' | 'idle' | 'stalled';
    }
    this.publish(owner, notify);
  }

  private publish(owner: Owner, notify: 'needs_input' | 'idle' | 'stalled' | null): void {
    this.deps.setBadge(this.waitingCount());
    this.deps.notify({
      type: 'agent',
      projectId: owner.projectId,
      workspaceId: owner.workspaceId,
      title: owner.title,
      notify,
    });
  }
}
