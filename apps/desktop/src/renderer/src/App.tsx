import type { Project, Workspace, WorkspaceStatus } from '@studio/schema';
import { QueryClientProvider, useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { TRPCClientError } from '@trpc/client';
import type { ReactNode } from 'react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isWaiting, useAgentNotifications, useAgentStatuses, useReportView } from './agent/status';
import { type Language, languages, setLanguage } from './i18n';
import { useStudioEvents } from './lib/events';
import { queryClient, TRPCProvider, trpcClient, useTRPC } from './lib/trpc';
import { PainButton, PainDialog, usePain } from './pain/Pain';
import { ProjectView } from './project/ProjectView';
import { applyTheme, storedTheme, swatches, type Theme, themes } from './theme';
import { WorkspaceList } from './workspace/WorkspaceList';
import { WorkspaceView } from './workspace/WorkspaceView';

const steps = ['task', 'plan', 'run', 'verify', 'review'] as const;

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <TRPCProvider trpcClient={trpcClient} queryClient={queryClient}>
        <Shell />
      </TRPCProvider>
    </QueryClientProvider>
  );
}

/** A selection that survives restarts: the open project, then the open workspace in it. */
function useStoredId(key: string): [string | null, (id: string | null) => void] {
  const [id, setId] = useState<string | null>(() => {
    try {
      return localStorage.getItem(key);
    } catch {
      return null;
    }
  });
  const select = (next: string | null) => {
    setId(next);
    try {
      if (next) localStorage.setItem(key, next);
      else localStorage.removeItem(key);
    } catch {
      // Not persisted; the selection still applies for this session.
    }
  };
  return [id, select];
}

function Shell() {
  useStudioEvents();
  const trpc = useTRPC();
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [selectedId, setSelectedId] = useStoredId('studio.selectedProject');
  const [workspaceId, selectWorkspace] = useStoredId('studio.selectedWorkspace');
  // Shown once on the workspace that was just created without a fresh origin/<main>.
  const [fetchError, setFetchError] = useState<{ workspaceId: string; error: string } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const select = (id: string | null) => {
    setSelectedId(id);
    selectWorkspace(null);
  };

  const projects = useQuery(trpc.projects.list.queryOptions());
  const info = useQuery(trpc.system.info.queryOptions());
  const pickFolder = useMutation(trpc.projects.pickFolder.mutationOptions());
  const add = useMutation(trpc.projects.add.mutationOptions());
  const remove = useMutation(trpc.projects.remove.mutationOptions());

  const claudeInstalled = info.data?.claude.installed ?? true;
  const selected = projects.data?.find((p) => p.id === selectedId) ?? null;
  const workspaces = useQuery({
    ...trpc.workspaces.list.queryOptions({ projectId: selected?.id ?? '' }),
    enabled: Boolean(selected),
  });
  const openWorkspace = selected ? workspaces.data?.find((w) => w.workspace.id === workspaceId) : undefined;

  // D3: which agents wait for the developer, OS notifications, and what is being looked at.
  const statuses = useAgentStatuses();
  const waiting = [...statuses.values()].filter(isWaiting);
  const openAt = (projectId: string, id: string) => {
    setSelectedId(projectId);
    selectWorkspace(id);
  };
  useAgentNotifications(openAt, info.data?.notifications ?? false);
  useReportView(openWorkspace?.workspace.id ?? null);

  // D5: the "Pain" box, from the sidebar button or the hotkey anywhere.
  const pain = usePain({
    projectId: selected?.id ?? null,
    projectName: selected?.name ?? null,
    workspaceId: openWorkspace?.workspace.id ?? null,
    workspaceName: openWorkspace?.workspace.slug ?? null,
  });

  async function addProject() {
    setError(null);
    try {
      const path = await pickFolder.mutateAsync();
      if (!path) return;
      // The project opens on its task box: writing a task is the first step, the map is built on demand.
      const { project } = await add.mutateAsync({ path });
      await queryClient.invalidateQueries({ queryKey: trpc.projects.list.queryKey() });
      select(project.id);
    } catch (e) {
      setError(
        e instanceof TRPCClientError && e.message === 'not_a_git_repo'
          ? t('errors.notAGitRepo')
          : t('errors.generic', { message: e instanceof Error ? e.message : String(e) }),
      );
    }
  }

  async function removeProject(project: Project) {
    await remove.mutateAsync({ projectId: project.id });
    if (selectedId === project.id) select(null);
    await queryClient.invalidateQueries({ queryKey: trpc.projects.list.queryKey() });
  }

  return (
    <div className="flex h-full">
      <Sidebar
        projects={projects.data ?? []}
        selectedId={selected?.id ?? null}
        onSelect={select}
        onAdd={addProject}
        adding={pickFolder.isPending || add.isPending}
        workspaceId={openWorkspace?.workspace.id ?? null}
        onSelectWorkspace={selectWorkspace}
        statuses={statuses}
        waiting={waiting}
        onOpenWaiting={(s) => openAt(s.projectId, s.workspaceId)}
        onPain={pain.show}
        onWorkspaceCreated={(workspace, error) => {
          setFetchError(error ? { workspaceId: workspace.id, error } : null);
          selectWorkspace(workspace.id);
        }}
      />
      <main className="flex min-w-0 flex-1 flex-col">
        <div className="drag h-10 shrink-0 border-b border-line" />
        {error && (
          <div
            role="alert"
            className="flex items-start gap-3 border-b border-remove/30 bg-panel px-4 py-2 text-[12.5px]"
          >
            <span className="flex-1 select-text">{error}</span>
            <button
              type="button"
              onClick={() => setError(null)}
              className="text-muted hover:text-ink"
              aria-label="✕"
            >
              ✕
            </button>
          </div>
        )}
        <div className="min-h-0 flex-1 overflow-auto">
          {selected && openWorkspace ? (
            <WorkspaceView
              key={openWorkspace.workspace.id}
              project={selected}
              workspace={openWorkspace.workspace}
              missing={openWorkspace.missing}
              fetchError={fetchError?.workspaceId === openWorkspace.workspace.id ? fetchError.error : null}
              onRemoved={() => selectWorkspace(null)}
            />
          ) : selected ? (
            <ProjectView
              key={selected.id}
              project={selected}
              claudeInstalled={claudeInstalled}
              onRemove={() => void removeProject(selected)}
            />
          ) : (
            <Welcome onAdd={addProject} />
          )}
        </div>
        <StatusBar />
      </main>
      <PainDialog open={pain.open} onClose={pain.close} context={pain.context} />
    </div>
  );
}

function Sidebar(props: {
  projects: Project[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  onAdd: () => void;
  adding: boolean;
  workspaceId: string | null;
  onSelectWorkspace: (id: string) => void;
  onWorkspaceCreated: (workspace: Workspace, fetchError: string | null) => void;
  statuses: Map<string, WorkspaceStatus>;
  waiting: WorkspaceStatus[];
  onOpenWaiting: (status: WorkspaceStatus) => void;
  onPain: () => void;
}) {
  const { t } = useTranslation();
  const first = props.waiting[0];
  return (
    <aside className="flex w-60 shrink-0 flex-col border-r border-line bg-bg">
      {/* Leaves room for the macOS window buttons. */}
      <div className="drag flex h-10 shrink-0 items-center justify-between pr-2 pl-20">
        <span className="text-[13px] font-semibold tracking-tight">Studio</span>
        {first && (
          <button
            type="button"
            title={t('agent.waitingHint')}
            onClick={() => props.onOpenWaiting(first)}
            className="no-drag flex items-center gap-1.5 rounded-full border border-modify/60 px-2 py-0.5 text-[11.5px] font-medium text-modify hover:bg-modify/10"
          >
            <span className="h-1.5 w-1.5 rounded-full bg-modify" />
            {t('agent.waiting', { count: props.waiting.length })}
          </button>
        )}
      </div>
      <div className="flex items-center justify-between px-3 pt-3 pb-1">
        <span className="text-[12px] font-medium text-muted">{t('sidebar.projects')}</span>
        <button
          type="button"
          onClick={props.onAdd}
          disabled={props.adding}
          title={t('sidebar.addProject')}
          aria-label={t('sidebar.addProject')}
          className="grid h-5 w-5 place-items-center rounded text-muted hover:bg-sel hover:text-ink disabled:opacity-40"
        >
          <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
            <path d="M6 1.5v9M1.5 6h9" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
          </svg>
        </button>
      </div>
      {props.projects.length === 0 ? (
        <p className="px-3 py-1 text-[12px] text-faint">{t('sidebar.noProjects')}</p>
      ) : (
        <ul className="min-h-0 flex-1 overflow-auto px-1.5">
          {props.projects.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                onClick={() => props.onSelect(p.id)}
                aria-current={p.id === props.selectedId && !props.workspaceId ? 'page' : undefined}
                title={p.repoPath}
                className="flex w-full items-center gap-1.5 rounded-md px-1.5 py-1 text-left text-[13px] text-muted hover:bg-panel hover:text-ink aria-[current=page]:bg-sel aria-[current=page]:text-ink"
              >
                <span className="min-w-0 flex-1 truncate">{p.name}</span>
                {p.id !== props.selectedId && props.waiting.some((s) => s.projectId === p.id) && (
                  <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-modify" />
                )}
              </button>
              {p.id === props.selectedId && (
                <WorkspaceList
                  projectId={p.id}
                  selectedId={props.workspaceId}
                  onSelect={props.onSelectWorkspace}
                  onCreated={props.onWorkspaceCreated}
                  statuses={props.statuses}
                />
              )}
            </li>
          ))}
        </ul>
      )}
      <div className="mt-auto space-y-3 border-t border-line p-3">
        <PainButton onClick={props.onPain} />
        <Preferences />
      </div>
    </aside>
  );
}

function Preferences() {
  const { t, i18n } = useTranslation();
  const [theme, setThemeState] = useState<Theme>(storedTheme);
  return (
    <>
      <Segmented
        label={t('settings.language')}
        value={i18n.language as Language}
        options={languages.map((l) => ({ value: l, label: l.toUpperCase() }))}
        onChange={setLanguage}
      />
      <ThemePicker
        value={theme}
        onChange={(v) => {
          applyTheme(v);
          setThemeState(v);
        }}
      />
    </>
  );
}

function ThemePicker(props: { value: Theme; onChange: (theme: Theme) => void }) {
  const { t } = useTranslation();
  const label = (theme: Theme) => t(`settings.themes.${theme}`);
  return (
    <fieldset className="space-y-1">
      <legend className="text-[11px] text-faint">
        {t('settings.theme')} <span className="text-muted">{label(props.value)}</span>
      </legend>
      <div className="flex justify-between">
        {themes.map((theme) => (
          <label
            key={theme}
            title={label(theme)}
            className="grid h-6 w-6 cursor-default place-items-center rounded-full ring-1 ring-line has-checked:ring-2 has-checked:ring-accent has-focus-visible:outline-2 has-focus-visible:outline-offset-2 has-focus-visible:outline-accent"
          >
            <input
              type="radio"
              name="theme"
              value={theme}
              checked={theme === props.value}
              onChange={() => props.onChange(theme)}
              aria-label={label(theme)}
              className="sr-only"
            />
            <Swatch theme={theme} />
          </label>
        ))}
      </div>
    </fieldset>
  );
}

// A theme's background with its accent as a dot; "system" is split between Light and Dark.
function Swatch({ theme }: { theme: Theme }) {
  if (theme === 'system') {
    return (
      <span
        aria-hidden="true"
        className="h-5 w-5 rounded-full"
        style={{
          background: `linear-gradient(135deg, ${swatches.light.bg} 50%, ${swatches.dark.bg} 50%)`,
          boxShadow: 'inset 0 0 0 1px rgb(128 128 128 / 0.35)',
        }}
      />
    );
  }
  const { bg, accent } = swatches[theme];
  return (
    <span
      aria-hidden="true"
      className="grid h-5 w-5 place-items-center rounded-full"
      style={{ background: bg, boxShadow: 'inset 0 0 0 1px rgb(128 128 128 / 0.35)' }}
    >
      <span className="h-2 w-2 rounded-full" style={{ background: accent }} />
    </span>
  );
}

function Segmented<T extends string>(props: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
}) {
  return (
    <fieldset className="space-y-1">
      <legend className="text-[11px] text-faint">{props.label}</legend>
      <div className="flex rounded-md border border-line bg-panel p-0.5">
        {props.options.map((o) => (
          <label
            key={o.value}
            className="flex-1 cursor-default rounded px-1.5 py-0.5 text-center text-[12px] text-muted has-checked:bg-sel has-checked:text-ink has-focus-visible:outline-2 has-focus-visible:outline-accent"
          >
            <input
              type="radio"
              name={props.label}
              value={o.value}
              checked={o.value === props.value}
              onChange={() => props.onChange(o.value)}
              className="sr-only"
            />
            {o.label}
          </label>
        ))}
      </div>
    </fieldset>
  );
}

function Welcome({ onAdd }: { onAdd: () => void }) {
  const { t } = useTranslation();
  return (
    <section className="flex min-h-full items-center px-10 py-12">
      <div className="mx-auto w-full max-w-3xl">
        <h1 className="max-w-xl text-[22px] font-semibold leading-tight tracking-tight">
          {t('welcome.title')}
        </h1>
        <p className="mt-3 max-w-xl text-[14px] leading-relaxed text-muted">{t('welcome.lead')}</p>

        <ol className="mt-8 grid grid-cols-5 border-y border-line">
          {steps.map((step, i) => (
            <Step key={step} index={i + 1} title={t(`welcome.steps.${step}.title`)}>
              {t(`welcome.steps.${step}.text`)}
            </Step>
          ))}
        </ol>

        <div className="mt-8 flex max-w-xl items-start gap-4">
          <button
            type="button"
            onClick={onAdd}
            className="shrink-0 rounded-md border border-accent px-3 py-1.5 text-[13px] font-medium text-accent-fg hover:bg-accent/10"
          >
            {t('welcome.addProject')}
          </button>
          <span className="text-[12px] leading-snug text-muted">{t('welcome.hint')}</span>
        </div>
      </div>
    </section>
  );
}

function Step(props: { index: number; title: string; children: ReactNode }) {
  return (
    <li className="border-line py-3 pr-3 not-first:border-l not-first:pl-3">
      <div className="flex items-baseline gap-1.5">
        <span className="font-mono text-[11px] text-faint">{props.index}</span>
        <span className="text-[13px] font-medium">{props.title}</span>
      </div>
      <p className="mt-1 text-[12px] leading-snug text-muted">{props.children}</p>
    </li>
  );
}

function StatusBar() {
  const { t } = useTranslation();
  const { electron, node } = window.studio.versions;
  return (
    <footer className="flex h-6 shrink-0 items-center border-t border-line px-3 text-[11px] text-faint">
      {t('status.runtime', { electron, node })}
    </footer>
  );
}
