import type { Project, Workspace } from '@studio/schema';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type ReactNode, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { StatusDot, useAgentStatuses } from '../agent/status';
import { useTRPC } from '../lib/trpc';
import { ChangesPanel } from '../workspace/ChangesPanel';
import { TerminalPane } from './TerminalPane';
import { disposeXterm } from './xterm-registry';

const SETUP_TAB = 'setup';
const CHANGES_TAB = 'changes';
/** The open tab of each workspace, kept while the app runs. */
const activeTabs = new Map<string, string>();

/** The first word of the agent command, without its path: "claude", "codex"… */
export function agentLabel(command: string): string {
  const first = command.trim().split(/\s+/)[0] ?? '';
  return first.split('/').pop() || first;
}

/** Terminal tabs of a workspace (agent first, then shells) and the setup output as the last tab. */
export function WorkspaceTabs(props: {
  project: Project;
  workspace: Workspace;
  missing: boolean;
  setup: ReactNode;
}) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { project, workspace } = props;
  const listKey = trpc.terminals.list.queryKey({ workspaceId: workspace.id });
  const list = useQuery(trpc.terminals.list.queryOptions({ workspaceId: workspace.id }));
  const defaults = useQuery(trpc.projects.openDefaults.queryOptions());
  const refresh = () => queryClient.invalidateQueries({ queryKey: listKey });
  // The answer is the list itself: a list fetch still in flight from before must not win with no tabs.
  const ensure = useMutation(
    trpc.terminals.ensureDefaults.mutationOptions({
      onSuccess: (tabs) => {
        queryClient.setQueryData(listKey, tabs);
      },
    }),
  );
  const addShell = useMutation(
    trpc.terminals.addShell.mutationOptions({
      onSuccess: async (terminal) => {
        await refresh();
        select(terminal.id);
      },
    }),
  );
  const close = useMutation(trpc.terminals.close.mutationOptions({ onSuccess: refresh }));
  const [active, setActive] = useState<string | null>(() => activeTabs.get(workspace.id) ?? null);
  const select = (id: string) => {
    activeTabs.set(workspace.id, id);
    setActive(id);
  };

  // A workspace opened for the first time gets the agent and a shell.
  // biome-ignore lint/correctness/useExhaustiveDependencies: once per workspace.
  useEffect(() => {
    if (!props.missing) ensure.mutate({ workspaceId: workspace.id });
  }, [workspace.id, props.missing]);

  const terminals = list.data ?? [];
  const status = useAgentStatuses().get(workspace.id);
  const hasSetup = workspace.setupStatus !== 'none' || Boolean(project.setupCommand);
  const agent = agentLabel(project.agentCommand ?? defaults.data?.agent ?? 'claude');
  const shellNumber = new Map(
    terminals.filter((x) => x.terminal.kind === 'shell').map((x, i) => [x.terminal.id, i + 1]),
  );
  const label = (id: string, kind: 'agent' | 'shell') => {
    if (kind === 'agent') return agent;
    const n = shellNumber.get(id) ?? 1;
    return n === 1 ? 'shell' : `shell ${n}`;
  };

  const ids = [...terminals.map((x) => x.terminal.id), CHANGES_TAB, ...(hasSetup ? [SETUP_TAB] : [])];
  // The agent tab opens by default; Changes only on request (or when there are no terminals at all).
  const current =
    active && ids.includes(active)
      ? active
      : (terminals[0]?.terminal.id ?? (list.isSuccess ? CHANGES_TAB : null));
  const open = terminals.find((x) => x.terminal.id === current);

  const tab = (id: string, text: string, extra?: ReactNode) => (
    <div
      key={id}
      className="group flex items-center rounded-t-md border border-b-0 border-transparent data-[on=true]:border-line data-[on=true]:bg-bg"
      data-on={id === current}
    >
      <button
        type="button"
        role="tab"
        aria-selected={id === current}
        onClick={() => select(id)}
        className="px-3 py-1 font-mono text-[12px] text-muted group-data-[on=true]:text-ink hover:text-ink"
      >
        {text}
      </button>
      {extra}
    </div>
  );

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div role="tablist" className="flex items-end gap-0.5 border-b border-line bg-panel px-3 pt-1.5">
        {terminals.map(({ terminal, process }) =>
          tab(
            terminal.id,
            label(terminal.id, terminal.kind),
            <>
              {terminal.kind === 'agent' && process.state === 'running' && (
                <span className="mr-2.5 flex">
                  <StatusDot status={status} />
                </span>
              )}
              {process.state === 'exited' && <span className="mr-1 text-[10px] text-faint">✕</span>}
              {terminal.kind === 'shell' && (
                <button
                  type="button"
                  aria-label={t('terminal.close', { name: label(terminal.id, terminal.kind) })}
                  title={t('terminal.close', { name: label(terminal.id, terminal.kind) })}
                  onClick={() => {
                    disposeXterm(terminal.id);
                    close.mutate({ terminalId: terminal.id });
                  }}
                  className="mr-1 grid h-4 w-4 place-items-center rounded text-[11px] text-faint opacity-0 group-hover:opacity-100 hover:bg-sel hover:text-ink"
                >
                  ×
                </button>
              )}
            </>,
          ),
        )}
        {!props.missing && (
          <button
            type="button"
            onClick={() => addShell.mutate({ workspaceId: workspace.id })}
            title={t('terminal.newShell')}
            aria-label={t('terminal.newShell')}
            className="mb-0.5 ml-1 grid h-6 w-6 place-items-center rounded text-muted hover:bg-sel hover:text-ink"
          >
            +
          </button>
        )}
        <div className="flex-1" />
        {tab(CHANGES_TAB, t('changes.title'))}
        {hasSetup &&
          tab(
            SETUP_TAB,
            t('workspace.setup.title'),
            workspace.setupStatus === 'running' ? (
              <span className="mr-2 h-1.5 w-1.5 animate-pulse rounded-full bg-accent motion-reduce:animate-none" />
            ) : workspace.setupStatus === 'failed' ? (
              <span className="mr-2 h-1.5 w-1.5 rounded-full bg-remove" />
            ) : undefined,
          )}
      </div>
      <div className="min-h-0 flex-1">
        {current === SETUP_TAB ? (
          <div className="h-full overflow-auto px-8 pb-8">{props.setup}</div>
        ) : current === CHANGES_TAB ? (
          <div className="h-full overflow-auto px-6">
            <ChangesPanel project={project} workspace={workspace} />
          </div>
        ) : open ? (
          <TerminalPane
            key={open.terminal.id}
            terminal={open.terminal}
            process={open.process}
            label={label(open.terminal.id, open.terminal.kind)}
          />
        ) : null}
      </div>
    </div>
  );
}
