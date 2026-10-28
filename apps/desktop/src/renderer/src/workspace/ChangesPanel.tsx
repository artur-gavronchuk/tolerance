import type { Project, Workspace } from '@studio/schema';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { type FormEvent, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { DiffText, splitPatch } from '../diff/DiffText';
import { useTRPC } from '../lib/trpc';
import { Notice } from '../project/shared';
import { userError } from './shared';

/** How often the open Changes tab re-reads git: the agent edits files in the background. */
const REFRESH_MS = 3000;

/**
 * The workspace branch against the main branch: what changed (committed and not), a commit with the
 * developer's message, push, and the pull request. Nothing is ever merged into the main branch.
 */
export function ChangesPanel({ project, workspace }: { project: Project; workspace: Workspace }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const input = { workspaceId: workspace.id };
  const live = { refetchInterval: REFRESH_MS, refetchOnWindowFocus: true, staleTime: 0 };
  const state = useQuery({ ...trpc.changes.state.queryOptions(input), ...live });
  const diff = useQuery({ ...trpc.changes.diff.queryOptions(input), ...live });
  const pr = useQuery({ ...trpc.changes.pullRequest.queryOptions(input), staleTime: 60_000 });
  const [message, setMessage] = useState('');
  const [openFile, setOpenFile] = useState<string | null>(null);

  const refresh = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: trpc.changes.state.queryKey(input) }),
      queryClient.invalidateQueries({ queryKey: trpc.changes.diff.queryKey(input) }),
      queryClient.invalidateQueries({ queryKey: trpc.changes.pullRequest.queryKey(input) }),
    ]);
  };
  const commit = useMutation(
    trpc.changes.commit.mutationOptions({
      onSuccess: async () => {
        setMessage('');
        await refresh();
      },
    }),
  );
  const push = useMutation(trpc.changes.push.mutationOptions({ onSuccess: refresh }));
  const createPr = useMutation(
    trpc.changes.createPullRequest.mutationOptions({
      onSuccess: async (url) => {
        await refresh();
        window.open(url);
      },
    }),
  );

  const s = state.data;
  const busy = commit.isPending || push.isPending || createPr.isPending;
  const canPush = Boolean(s && s.commits > 0 && (s.upstream === null || (s.unpushed ?? 0) > 0));
  const error = commit.error ?? push.error ?? createPr.error;

  function submit(e?: FormEvent) {
    e?.preventDefault();
    if (message.trim() && s && s.uncommitted > 0 && !busy) commit.mutate({ ...input, message });
  }

  const files = diff.data?.files ?? [];
  const patches = diff.data ? splitPatch(diff.data.patch) : new Map<string, string>();
  const shownPatch = openFile ? (patches.get(openFile) ?? '') : (diff.data?.patch ?? '');

  return (
    <div className="space-y-4 py-4 text-[12.5px]">
      {state.error && <Notice tone="error">{userError(t, state.error)}</Notice>}
      {s && (
        <p className="text-muted" data-testid="branch-state">
          {[
            t('changes.uncommitted', { count: s.uncommitted }),
            t('changes.commits', { count: s.commits, base: workspace.baseRef }),
            s.upstream === null
              ? t('changes.notPushed')
              : s.unpushed
                ? t('changes.unpushed', { count: s.unpushed })
                : t('changes.pushed'),
          ].join(' · ')}
        </p>
      )}

      <form onSubmit={submit} className="rounded-lg border border-line bg-raised focus-within:border-accent">
        <label htmlFor="commit-message" className="sr-only">
          {t('changes.message')}
        </label>
        <textarea
          id="commit-message"
          rows={2}
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit();
          }}
          placeholder={t('changes.messagePlaceholder')}
          className="block w-full resize-y bg-transparent px-3 pt-2 pb-1 text-[13px] placeholder:text-faint"
          style={{ outline: 'none' }}
        />
        <div className="flex flex-wrap items-center gap-2 px-3 pb-2">
          <button
            type="submit"
            disabled={busy || !message.trim() || !s || s.uncommitted === 0}
            className="rounded-md border border-accent px-3 py-1 text-[12.5px] font-medium text-accent-fg hover:bg-accent/10 disabled:opacity-50"
          >
            {t('changes.commit')}
          </button>
          <button
            type="button"
            disabled={busy || !canPush}
            onClick={() => push.mutate(input)}
            className="rounded-md border border-line px-3 py-1 text-[12.5px] hover:border-accent hover:text-accent-fg disabled:opacity-50"
          >
            {push.isPending ? t('changes.pushing') : t('changes.push')}
          </button>
          {pr.data?.url ? (
            <a
              href={pr.data.url}
              target="_blank"
              rel="noreferrer"
              className="rounded-md border border-add/60 px-3 py-1 text-[12.5px] text-add hover:bg-add/10"
            >
              {t('changes.openPr', { number: /\/pull\/(\d+)/.exec(pr.data.url)?.[1] ?? '' })}
            </a>
          ) : (
            pr.data?.gh === 'ok' && (
              <button
                type="button"
                disabled={busy || !s || s.commits === 0}
                onClick={() => createPr.mutate(input)}
                className="rounded-md border border-line px-3 py-1 text-[12.5px] hover:border-accent hover:text-accent-fg disabled:opacity-50"
              >
                {createPr.isPending
                  ? t('changes.creatingPr')
                  : t('changes.createPr', { base: project.defaultBranch })}
              </button>
            )
          )}
          <span className="ml-auto text-[11px] text-faint">{t('changes.hint')}</span>
        </div>
      </form>

      {error && <Notice tone="error">{userError(t, error)}</Notice>}
      {pr.data && pr.data.gh !== 'ok' && !pr.data.url && (
        <Notice>
          <p>{pr.data.gh === 'missing' ? t('changes.ghMissing') : t('changes.ghLoggedOut')}</p>
          <pre className="mt-1.5 font-mono text-[11.5px] whitespace-pre-wrap text-ink select-text">
            {pr.data.commands.join('\n')}
          </pre>
          {pr.data.compareUrl && (
            <a
              href={pr.data.compareUrl}
              target="_blank"
              rel="noreferrer"
              className="mt-1.5 inline-block text-accent-fg underline"
            >
              {t('changes.compare')}
            </a>
          )}
        </Notice>
      )}

      {diff.data && files.length === 0 && <p className="text-muted">{t('changes.empty')}</p>}
      {files.length > 0 && (
        <div className="grid grid-cols-[minmax(180px,260px)_1fr] gap-3">
          <ul className="min-w-0 space-y-0.5 font-mono text-[11.5px]" aria-label={t('changes.files')}>
            <li>
              <button
                type="button"
                aria-current={openFile === null ? 'true' : undefined}
                onClick={() => setOpenFile(null)}
                className="w-full rounded px-1.5 py-0.5 text-left font-sans text-[12px] text-muted hover:bg-panel aria-[current=true]:bg-sel aria-[current=true]:text-ink"
              >
                {t('changes.allFiles', { count: files.length })}
              </button>
            </li>
            {files.map((f) => (
              <li key={f.path}>
                <button
                  type="button"
                  title={f.path}
                  aria-current={openFile === f.path ? 'true' : undefined}
                  onClick={() => setOpenFile(f.path)}
                  className="flex w-full items-center gap-1.5 rounded px-1.5 py-0.5 text-left hover:bg-panel aria-[current=true]:bg-sel"
                >
                  <span className="w-3 shrink-0 font-sans text-[10.5px] text-faint">
                    {f.status === 'added' ? 'A' : f.status === 'deleted' ? 'D' : 'M'}
                  </span>
                  <span className="min-w-0 flex-1 truncate">{f.path}</span>
                  {f.additions !== null && f.additions > 0 && (
                    <span className="text-add">+{f.additions}</span>
                  )}
                  {f.deletions !== null && f.deletions > 0 && (
                    <span className="text-remove">−{f.deletions}</span>
                  )}
                  {f.additions === null && <span className="font-sans text-[10.5px] text-faint">bin</span>}
                </button>
              </li>
            ))}
          </ul>
          <div className="min-w-0" data-testid="changes-diff">
            <DiffText patch={shownPatch} maxHeight={false} />
            {diff.data?.truncated && <p className="mt-1 text-modify">{t('exec.truncated')}</p>}
          </div>
        </div>
      )}
    </div>
  );
}
