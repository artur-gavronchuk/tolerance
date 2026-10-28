import type { Workspace, WorkspaceStatus } from '@studio/schema';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type FormEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { StatusDot } from '../agent/status';
import { useTRPC } from '../lib/trpc';
import { relativeTime, useNow, userError } from './shared';

/** The selected project's workspaces in the sidebar, newest first, with a "+" to create one. */
export function WorkspaceList(props: {
  projectId: string;
  selectedId: string | null;
  onSelect: (id: string) => void;
  onCreated: (workspace: Workspace, fetchError: string | null) => void;
  statuses: Map<string, WorkspaceStatus>;
}) {
  const trpc = useTRPC();
  const { t, i18n } = useTranslation();
  const now = useNow();
  const list = useQuery(trpc.workspaces.list.queryOptions({ projectId: props.projectId }));
  const [adding, setAdding] = useState(false);

  return (
    <div className="mt-1 mb-1 ml-2 border-l border-line pl-1.5">
      <div className="flex items-center justify-between py-0.5 pr-1 pl-1.5">
        <span className="text-[11px] text-faint">{t('sidebar.workspaces')}</span>
        <button
          type="button"
          onClick={() => setAdding(true)}
          title={t('sidebar.newWorkspace')}
          aria-label={t('sidebar.newWorkspace')}
          className="grid h-5 w-5 place-items-center rounded text-muted hover:bg-sel hover:text-ink"
        >
          <svg width="10" height="10" viewBox="0 0 12 12" aria-hidden="true">
            <path d="M6 1.5v9M1.5 6h9" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      </div>
      {adding && (
        <NewWorkspace
          projectId={props.projectId}
          onDone={(workspace, fetchError) => {
            setAdding(false);
            props.onCreated(workspace, fetchError);
          }}
          onCancel={() => setAdding(false)}
        />
      )}
      {list.data?.length === 0 && !adding && (
        <p className="px-1.5 py-0.5 text-[11.5px] text-faint">{t('sidebar.noWorkspaces')}</p>
      )}
      <ul>
        {list.data?.map(({ workspace, missing }) => {
          const note = missing
            ? t('sidebar.missing')
            : workspace.setupStatus === 'running'
              ? t('sidebar.setupRunning')
              : workspace.setupStatus === 'failed'
                ? t('sidebar.setupFailed')
                : null;
          return (
            <li key={workspace.id}>
              <button
                type="button"
                onClick={() => props.onSelect(workspace.id)}
                aria-current={workspace.id === props.selectedId ? 'page' : undefined}
                title={workspace.path}
                className="w-full rounded-md px-1.5 py-1 text-left text-muted hover:bg-panel hover:text-ink aria-[current=page]:bg-sel aria-[current=page]:text-ink"
              >
                <span className="flex items-center gap-1.5">
                  <span className="min-w-0 flex-1 truncate text-[12.5px]">{workspace.name}</span>
                  <StatusDot status={props.statuses.get(workspace.id)} />
                  {workspace.setupStatus === 'running' && (
                    <span className="h-1.5 w-1.5 shrink-0 animate-pulse rounded-full bg-accent motion-reduce:animate-none" />
                  )}
                </span>
                <span className="flex items-baseline gap-1.5 text-[10.5px] text-faint">
                  <span className="min-w-0 truncate font-mono">
                    {workspace.branch.replace(/^studio\/ws\//, '')}
                  </span>
                  <span className="ml-auto shrink-0">
                    {relativeTime(workspace.createdAt, now, i18n.language)}
                  </span>
                </span>
                {note && (
                  <span
                    className={`block text-[10.5px] ${workspace.setupStatus === 'failed' || missing ? 'text-remove' : 'text-faint'}`}
                  >
                    {note}
                  </span>
                )}
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function NewWorkspace(props: {
  projectId: string;
  onDone: (workspace: Workspace, fetchError: string | null) => void;
  onCancel: () => void;
}) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [name, setName] = useState('');
  const create = useMutation(
    trpc.workspaces.create.mutationOptions({
      onSuccess: async (result) => {
        await queryClient.invalidateQueries({
          queryKey: trpc.workspaces.list.queryKey({ projectId: props.projectId }),
        });
        props.onDone(result.workspace, result.fetchError);
      },
    }),
  );

  function submit(e?: FormEvent) {
    e?.preventDefault();
    if (name.trim() && !create.isPending) create.mutate({ projectId: props.projectId, name: name.trim() });
  }

  return (
    <form onSubmit={submit} className="px-1 py-1">
      <label htmlFor="new-workspace" className="sr-only">
        {t('sidebar.workspaceName')}
      </label>
      <input
        id="new-workspace"
        // biome-ignore lint/a11y/noAutofocus: the field appears because the user asked to type a name.
        autoFocus
        value={name}
        disabled={create.isPending}
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Escape') props.onCancel();
        }}
        onBlur={() => {
          if (!name.trim() && !create.isPending) props.onCancel();
        }}
        placeholder={t('sidebar.workspacePlaceholder')}
        className="w-full rounded-md border border-line bg-raised px-1.5 py-1 text-[12.5px] placeholder:text-faint focus:border-accent disabled:opacity-60"
        style={{ outline: 'none' }}
      />
      {create.isPending && <p className="mt-1 px-0.5 text-[11px] text-muted">{t('sidebar.creating')}</p>}
      {create.error && <p className="mt-1 px-0.5 text-[11px] text-remove">{userError(t, create.error)}</p>}
    </form>
  );
}
