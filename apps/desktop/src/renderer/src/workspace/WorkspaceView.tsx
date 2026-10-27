import type { Project, Workspace } from '@studio/schema';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTRPC } from '../lib/trpc';
import { Notice, Section } from '../project/shared';
import { WorkspaceTabs } from '../terminal/WorkspaceTabs';
import { disposeXterm } from '../terminal/xterm-registry';
import { relativeTime, useNow, userError } from './shared';

export function WorkspaceView(props: {
  project: Project;
  workspace: Workspace;
  missing: boolean;
  /** Set right after creation when `origin/<main>` could not be refreshed. */
  fetchError: string | null;
  onRemoved: () => void;
}) {
  const trpc = useTRPC();
  const { t, i18n } = useTranslation();
  const now = useNow();
  const { project, workspace } = props;
  const defaults = useQuery(trpc.projects.openDefaults.queryOptions());
  const open = useMutation(trpc.workspaces.open.mutationOptions());
  const [confirming, setConfirming] = useState(false);
  const terminals = useQuery(trpc.terminals.list.queryOptions({ workspaceId: workspace.id }));

  const openButton = (target: 'terminal' | 'editor' | 'folder', label: string, hint: string) => (
    <button
      type="button"
      title={hint}
      disabled={props.missing}
      onClick={() => open.mutate({ workspaceId: workspace.id, target })}
      className="rounded-md border border-line px-2.5 py-1 text-[12px] hover:border-accent hover:text-accent-fg disabled:opacity-50"
    >
      {label}
    </button>
  );

  const removed = async () => {
    // The tabs' xterms are kept for quick switching; a deleted workspace does not need them.
    for (const { terminal } of terminals.data ?? []) disposeXterm(terminal.id);
    props.onRemoved();
  };

  return (
    <div className="flex h-full flex-col">
      <header className="flex shrink-0 items-center justify-between gap-6 px-6 pt-3 pb-2.5">
        <div className="min-w-0" title={workspace.path}>
          <div className="flex min-w-0 items-baseline gap-2">
            <span className="shrink-0 text-[12px] text-faint">{project.name} /</span>
            <h1 className="truncate text-[16px] font-semibold tracking-tight">{workspace.name}</h1>
            <span className="truncate font-mono text-[12px] text-muted select-text">{workspace.branch}</span>
          </div>
          <p className="mt-0.5 truncate text-[11.5px] text-faint">
            {t('workspace.base', { ref: workspace.baseRef, sha: workspace.baseSha.slice(0, 7) })},{' '}
            {t('workspace.created', { time: relativeTime(workspace.createdAt, now, i18n.language) })} ·{' '}
            <span className="font-mono select-text">{workspace.path}</span>
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {openButton(
            'terminal',
            t('workspace.openTerminal'),
            t('workspace.openTerminalHint', { app: project.terminalApp ?? defaults.data?.terminal ?? '' }),
          )}
          {openButton(
            'editor',
            t('workspace.openEditor'),
            t('workspace.openEditorHint', { app: project.editorCommand ?? defaults.data?.editor ?? '' }),
          )}
          {openButton('folder', t('workspace.openFolder'), t('workspace.openFolderHint'))}
          <button
            type="button"
            title={t('workspace.removeHint')}
            onClick={() => setConfirming(true)}
            className="ml-2 rounded-md px-2 py-1 text-[12px] text-muted hover:bg-panel hover:text-remove"
          >
            {t('workspace.remove')}
          </button>
        </div>
      </header>

      {(open.error || props.missing || props.fetchError || confirming) && (
        <div className="shrink-0 px-6">
          {open.error && <Notice tone="error">{userError(t, open.error)}</Notice>}
          {props.missing && <Notice tone="warn">{t('workspace.missing')}</Notice>}
          {props.fetchError && (
            <Notice tone="warn">
              {t('workspace.fetchFailed', {
                branch: project.defaultBranch,
                ref: workspace.baseRef,
                error: props.fetchError,
              })}
            </Notice>
          )}
          {confirming && (
            <RemoveConfirm
              workspace={workspace}
              onCancel={() => setConfirming(false)}
              onRemoved={() => void removed()}
            />
          )}
        </div>
      )}

      <WorkspaceTabs
        project={project}
        workspace={workspace}
        missing={props.missing}
        setup={<SetupSection project={project} workspace={workspace} />}
      />
    </div>
  );
}

function RemoveConfirm(props: { workspace: Workspace; onCancel: () => void; onRemoved: () => void }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const { workspace } = props;
  // Always fresh: the user may have edited files since the last look.
  const changes = useQuery({
    ...trpc.workspaces.changes.queryOptions({ workspaceId: workspace.id }),
    staleTime: 0,
    gcTime: 0,
  });
  const remove = useMutation(trpc.workspaces.remove.mutationOptions({ onSuccess: props.onRemoved }));
  const shown = changes.data?.uncommitted.slice(0, 8) ?? [];
  const hidden = (changes.data?.uncommitted.length ?? 0) - shown.length;
  const risky = (changes.data?.uncommitted.length ?? 0) > 0;

  return (
    <div
      role="alertdialog"
      aria-labelledby="remove-title"
      className={`mb-3 rounded-lg border bg-panel px-3 py-2.5 text-[12.5px] ${risky ? 'border-remove/50' : 'border-line'}`}
    >
      <p id="remove-title" className="font-medium">
        {t('workspace.confirm.title', { name: workspace.name })}
      </p>
      {changes.isPending && <p className="mt-1 text-muted">{t('workspace.confirm.checking')}</p>}
      {changes.error && <p className="mt-1 text-remove">{userError(t, changes.error)}</p>}
      {changes.data && (
        <div className="mt-1 space-y-1 select-text">
          {changes.data.missing ? (
            <p>{t('workspace.confirm.gone')}</p>
          ) : risky ? (
            <>
              <p className="text-remove">
                {t('workspace.confirm.uncommitted', { count: changes.data.uncommitted.length })}
              </p>
              <ul className="font-mono text-[11.5px] text-muted">
                {shown.map((file) => (
                  <li key={file} className="truncate">
                    {file}
                  </li>
                ))}
                {hidden > 0 && <li>{t('workspace.confirm.more', { count: hidden })}</li>}
              </ul>
            </>
          ) : (
            <p>{t('workspace.confirm.clean')}</p>
          )}
          <p className="text-muted">
            {t('workspace.confirm.branchStays', { branch: workspace.branch })}
            {changes.data.unpushed > 0 &&
              ` ${t('workspace.confirm.unpushed', { count: changes.data.unpushed })}`}
          </p>
        </div>
      )}
      {remove.error && <p className="mt-1 text-remove">{userError(t, remove.error)}</p>}
      <div className="mt-2.5 flex gap-2">
        <button
          type="button"
          disabled={!changes.data || remove.isPending}
          onClick={() => remove.mutate({ workspaceId: workspace.id })}
          className="rounded-md border border-remove px-2.5 py-1 text-[12px] font-medium text-remove hover:bg-remove/10 disabled:opacity-50"
        >
          {t('workspace.confirm.remove')}
        </button>
        <button
          type="button"
          onClick={props.onCancel}
          className="rounded-md px-2.5 py-1 text-[12px] text-muted hover:bg-sel hover:text-ink"
        >
          {t('workspace.confirm.cancel')}
        </button>
      </div>
    </div>
  );
}

function SetupSection({ project, workspace }: { project: Project; workspace: Workspace }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const log = useQuery(trpc.workspaces.setupLog.queryOptions({ workspaceId: workspace.id }));
  const retry = useMutation(trpc.workspaces.retrySetup.mutationOptions());
  const pre = useRef<HTMLPreElement>(null);
  const status = workspace.setupStatus;

  // Follow the output while it grows, like a terminal does.
  // biome-ignore lint/correctness/useExhaustiveDependencies: scrolls whenever new output arrives.
  useEffect(() => {
    if (pre.current) pre.current.scrollTop = pre.current.scrollHeight;
  }, [log.data]);

  const label =
    status === 'running'
      ? t('workspace.setup.running')
      : status === 'done'
        ? t('workspace.setup.done')
        : status === 'failed'
          ? workspace.setupExitCode === null
            ? t('workspace.setup.failedNoCode')
            : t('workspace.setup.failed', { code: workspace.setupExitCode })
          : status === 'interrupted'
            ? t('workspace.setup.interrupted')
            : null;
  const tone = status === 'failed' ? 'text-remove' : status === 'done' ? 'text-add' : 'text-muted';
  const canRun = status !== 'running' && Boolean(project.setupCommand);

  return (
    <Section
      title={t('workspace.setup.title')}
      action={
        canRun && (
          <button
            type="button"
            disabled={retry.isPending}
            onClick={() => retry.mutate({ workspaceId: workspace.id })}
            className="rounded-md border border-line px-2.5 py-1 text-[12px] hover:border-accent hover:text-accent-fg disabled:opacity-50"
          >
            {status === 'none' ? t('workspace.setup.run') : t('workspace.setup.retry')}
          </button>
        )
      }
    >
      {retry.error && <Notice tone="error">{userError(t, retry.error)}</Notice>}
      {status === 'none' ? (
        <p className="text-[12.5px] text-muted">{t('workspace.setup.none')}</p>
      ) : (
        <>
          <p className={`mb-2 flex items-center gap-2 text-[12.5px] ${tone}`} aria-live="polite">
            {status === 'running' && (
              <span className="h-2 w-2 animate-pulse rounded-full bg-accent motion-reduce:animate-none" />
            )}
            <code className="font-mono text-[11.5px] text-muted">{project.setupCommand}</code>
            <span>{label}</span>
          </p>
          <pre
            ref={pre}
            data-testid="setup-log"
            className="max-h-[420px] min-h-16 overflow-auto rounded-md border border-line bg-panel px-3 py-2 font-mono text-[11.5px] leading-[1.45] whitespace-pre-wrap select-text"
          >
            {log.data || <span className="text-faint">{t('workspace.setup.empty')}</span>}
          </pre>
        </>
      )}
    </Section>
  );
}
