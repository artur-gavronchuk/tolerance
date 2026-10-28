import type { AgentStatus, WorkspaceStatus } from '@studio/schema';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useTRPC } from '../lib/trpc';

/** Agent status of every workspace that has a running (or finished) agent tab. */
export function useAgentStatuses(): Map<string, WorkspaceStatus> {
  const trpc = useTRPC();
  const statuses = useQuery(trpc.agents.statuses.queryOptions());
  return new Map((statuses.data ?? []).map((s) => [s.workspaceId, s]));
}

/** Waiting for the developer: asking right now, or something not looked at yet. */
export function isWaiting(status: WorkspaceStatus | undefined): boolean {
  return Boolean(status && (status.status === 'needs_input' || status.attention));
}

const dotClass: Record<AgentStatus, string> = {
  working: 'bg-accent animate-pulse motion-reduce:animate-none',
  needs_input: 'bg-modify',
  idle: 'bg-add',
  stalled: 'bg-remove',
  exited: 'bg-faint',
};

/**
 * A workspace's status dot. A finished turn already seen stays as a hollow ring: nothing to do there.
 */
export function StatusDot({ status }: { status: WorkspaceStatus | undefined }) {
  const { t } = useTranslation();
  if (!status) return null;
  const quiet = status.status === 'idle' && !status.attention;
  return (
    <span
      role="img"
      aria-label={t(`agent.status.${status.status}`)}
      title={t(`agent.status.${status.status}`)}
      data-status={status.status}
      data-attention={status.attention}
      className={`h-2 w-2 shrink-0 rounded-full ${quiet ? 'border border-add' : dotClass[status.status]}`}
    />
  );
}

/**
 * Tells main which workspace the developer is looking at: the open one while the window has focus.
 * That clears its mark and keeps notifications away for it.
 */
export function useReportView(workspaceId: string | null): void {
  const trpc = useTRPC();
  const view = useMutation(trpc.agents.view.mutationOptions());
  const report = useRef(view.mutate);
  report.current = view.mutate;

  useEffect(() => {
    const send = () => report.current({ workspaceId: document.hasFocus() ? workspaceId : null });
    send();
    window.addEventListener('focus', send);
    window.addEventListener('blur', send);
    return () => {
      window.removeEventListener('focus', send);
      window.removeEventListener('blur', send);
    };
    // A mark lands on the open workspace only while the window is away; the focus event clears it.
  }, [workspaceId]);
}

/**
 * OS notifications when an agent asks something, finishes or stalls in a workspace the developer is
 * not looking at (main decides that). A click brings Studio forward and opens the workspace.
 */
export function useAgentNotifications(
  open: (projectId: string, workspaceId: string) => void,
  enabled: boolean,
) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const focus = useMutation(trpc.system.focusWindow.mutationOptions());
  const latest = useRef({ open, focus: focus.mutate, t, enabled });
  latest.current = { open, focus: focus.mutate, t, enabled };

  useEffect(
    () =>
      window.studio.onEvent((event) => {
        const { open, focus, t, enabled } = latest.current;
        if (event.type !== 'agent' || !event.notify || !enabled || typeof Notification === 'undefined')
          return;
        const note = new Notification(event.title, {
          body: t(`agent.notify.${event.notify}`),
          silent: false,
        });
        note.onclick = () => {
          focus();
          open(event.projectId, event.workspaceId);
        };
      }),
    [],
  );
}
