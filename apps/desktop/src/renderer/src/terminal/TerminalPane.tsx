import type { Terminal, TerminalProcess } from '@studio/schema';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTRPC } from '../lib/trpc';
import { userError } from '../workspace/shared';
import { attach, detach, fitNow, xtermFor } from './xterm-registry';

/** Tabs started in this renderer session, so a remount (or React's double effect) never starts twice. */
const autoStarted = new Set<string>();

/**
 * Whether a tab starts by itself when shown. Shells always do. The agent does only in a new workspace
 * (no session yet); after an app restart it waits for a click rather than opening a fresh session.
 */
function startsByItself(terminal: Terminal, process: TerminalProcess): boolean {
  if (process.state !== 'never' || autoStarted.has(terminal.id)) return false;
  return terminal.kind === 'shell' || terminal.sessionId === null;
}

export function TerminalPane(props: { terminal: Terminal; process: TerminalProcess; label: string }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { terminal, process } = props;
  const host = useRef<HTMLDivElement>(null);
  const [searching, setSearching] = useState(false);
  const [query, setQuery] = useState('');
  const start = useMutation(
    trpc.terminals.start.mutationOptions({
      onSettled: () =>
        queryClient.invalidateQueries({
          queryKey: trpc.terminals.list.queryKey({ workspaceId: terminal.workspaceId }),
        }),
    }),
  );

  const entry = xtermFor(terminal.id, {
    loadSnapshot: () =>
      queryClient.fetchQuery({
        ...trpc.terminals.snapshot.queryOptions({ terminalId: terminal.id }),
        staleTime: 0,
      }),
    onSearch: () => setSearching(true),
    truncatedNote: t('terminal.truncated'),
  });

  const run = (resume = false) => {
    fitNow(entry);
    start.mutate({ terminalId: terminal.id, cols: entry.term.cols, rows: entry.term.rows, resume });
  };
  // An agent tab with a known Claude session continues it rather than starting over (D4).
  const resumable = terminal.kind === 'agent' && terminal.sessionId !== null;

  // Show the xterm here while the tab is open; park it when the tab or workspace is left.
  // biome-ignore lint/correctness/useExhaustiveDependencies: attach once per tab; the entry is stable for its id.
  useEffect(() => {
    const el = host.current;
    if (!el) return;
    attach(entry, el);
    entry.term.focus();
    let timer: ReturnType<typeof setTimeout> | null = null;
    // Fit at once and again when resizing stops: a trailing-only fit leaves the TUI drawing at the old size.
    const observer = new ResizeObserver(() => {
      if (!timer) fitNow(entry);
      if (timer) clearTimeout(timer);
      timer = setTimeout(() => {
        timer = null;
        fitNow(entry);
      }, 80);
    });
    observer.observe(el);
    if (startsByItself(terminal, process)) {
      autoStarted.add(terminal.id);
      void entry.ready.then(() => run());
    }
    return () => {
      observer.disconnect();
      if (timer) clearTimeout(timer);
      detach(entry);
    };
  }, [terminal.id]);

  const find = (backwards = false) => {
    if (!query) return;
    if (backwards) entry.search.findPrevious(query);
    else entry.search.findNext(query);
  };

  return (
    <div className="relative flex h-full min-h-0 flex-col">
      {searching && (
        <div className="absolute top-2 right-4 z-10 flex items-center gap-1 rounded-md border border-line bg-raised p-1 shadow-sm">
          <input
            // biome-ignore lint/a11y/noAutofocus: the field appears because the user pressed Cmd+F.
            autoFocus
            aria-label={t('terminal.search')}
            placeholder={t('terminal.search')}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              if (e.target.value) entry.search.findNext(e.target.value, { incremental: true });
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') find(e.shiftKey);
              if (e.key === 'Escape') {
                setSearching(false);
                entry.search.clearDecorations();
                entry.term.focus();
              }
            }}
            className="w-48 rounded bg-transparent px-1.5 py-0.5 text-[12px] placeholder:text-faint"
            style={{ outline: 'none' }}
          />
        </div>
      )}
      <div
        ref={host}
        data-testid={`terminal-${terminal.kind}`}
        className="min-h-0 flex-1 overflow-hidden bg-bg py-1 pl-3"
      />
      {process.state !== 'running' && !start.isPending && (
        <div className="flex items-center gap-3 border-t border-line bg-panel px-3 py-1.5 text-[12px]">
          <span className="text-muted">
            {process.state === 'exited'
              ? t('terminal.exited', { code: process.exitCode ?? '—' })
              : terminal.kind === 'agent'
                ? t('terminal.agentStopped', { agent: props.label })
                : t('terminal.stopped')}
          </span>
          {resumable && (
            <button
              type="button"
              onClick={() => run(true)}
              className="rounded-md border border-accent px-2 py-0.5 text-[12px] font-medium text-accent-fg hover:bg-accent/10"
            >
              {t('terminal.resume')}
            </button>
          )}
          <button
            type="button"
            onClick={() => run()}
            className="rounded-md border border-line px-2 py-0.5 text-[12px] hover:border-accent hover:text-accent-fg"
          >
            {terminal.kind === 'agent'
              ? resumable
                ? t('terminal.newSession')
                : t('terminal.startAgent', { agent: props.label })
              : t('terminal.start')}
          </button>
          {start.error && <span className="text-remove">{userError(t, start.error)}</span>}
        </div>
      )}
    </div>
  );
}
