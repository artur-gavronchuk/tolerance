import type {
  AgentEvent,
  ComponentKind,
  Project,
  RepoMap,
  RepoMapComponent,
  RepoScan,
  Run,
  ScanCategory,
  ScanFinding,
} from '@studio/schema';
import { useMutation, useQuery } from '@tanstack/react-query';
import { type ReactNode, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTRPC } from '../lib/trpc';

export function ProjectView(props: { project: Project; claudeInstalled: boolean; onRemove: () => void }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const scan = useQuery(trpc.projects.scan.queryOptions({ projectId: props.project.id }));

  return (
    <div className="mx-auto w-full max-w-5xl px-8 pt-6 pb-16">
      <header className="flex items-start justify-between gap-6">
        <div className="min-w-0">
          <h1 className="truncate text-[20px] font-semibold tracking-tight">{props.project.name}</h1>
          <p className="mt-0.5 truncate font-mono text-[12px] text-muted select-text">
            {props.project.repoPath}
          </p>
          {scan.data && (
            <p className="mt-1 text-[12px] text-faint">
              {[
                scan.data.branch ? t('project.branch', { branch: scan.data.branch }) : null,
                scan.data.commitSha ? scan.data.commitSha.slice(0, 7) : t('project.noCommits'),
                t('project.files', { count: scan.data.totalFiles }),
              ]
                .filter(Boolean)
                .join(', ')}
            </p>
          )}
        </div>
        <button
          type="button"
          title={t('project.removeHint')}
          onClick={props.onRemove}
          className="shrink-0 rounded-md px-2 py-1 text-[12px] text-muted hover:bg-panel hover:text-remove"
        >
          {t('project.remove')}
        </button>
      </header>

      {scan.data && <ScanSection scan={scan.data} />}
      {scan.error && (
        <p className="mt-6 text-[13px] text-remove">{t('errors.generic', { message: scan.error.message })}</p>
      )}

      <MapSection project={props.project} claudeInstalled={props.claudeInstalled} />
    </div>
  );
}

// ── quick scan ──────────────────────────────────────────────────────────────

const categories: ScanCategory[] = ['frontend', 'backend', 'database', 'infra', 'testing', 'docs'];

// Theme highlight colors (see styles.css), so charts follow the chosen theme.
const languagePalette = [
  'var(--hl-blue)',
  'var(--hl-purple)',
  'var(--hl-teal)',
  'var(--hl-amber)',
  'var(--hl-pink)',
  'var(--hl-slate)',
];

function ScanSection({ scan }: { scan: RepoScan }) {
  const { t } = useTranslation();
  const languages = scan.languages.filter((l) => l.share >= 0.005).slice(0, 6);

  return (
    <Section title={t('scan.title')} lead={t('scan.lead')}>
      {languages.length > 0 && (
        <div className="mb-5">
          <div className="flex h-1.5 overflow-hidden rounded-full bg-line" aria-hidden="true">
            {languages.map((l, i) => (
              <span key={l.name} style={{ width: `${l.share * 100}%`, background: languagePalette[i] }} />
            ))}
          </div>
          <ul className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-[12px]" aria-label={t('scan.languages')}>
            {languages.map((l, i) => (
              <li key={l.name} className="flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full" style={{ background: languagePalette[i] }} />
                <span>{l.name}</span>
                <span className="text-faint">{Math.round(l.share * 100)}%</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      <div className="grid grid-cols-3 gap-px overflow-hidden rounded-lg border border-line bg-line">
        {categories.map((category) => (
          <ScanCategoryBlock
            key={category}
            category={category}
            findings={scan.findings.filter((f) => f.category === category)}
          />
        ))}
      </div>
    </Section>
  );
}

function ScanCategoryBlock({ category, findings }: { category: ScanCategory; findings: ScanFinding[] }) {
  const { t } = useTranslation();
  return (
    <div className="bg-bg p-3">
      <h3 className="text-[12px] font-medium text-muted">{t(`scan.categories.${category}`)}</h3>
      {findings.length === 0 ? (
        <p className="mt-1.5 text-[12px] text-faint">{t('scan.empty')}</p>
      ) : (
        <ul className="mt-1.5 space-y-1">
          {findings.map((f) => (
            <li
              key={f.label}
              className="flex min-w-0 items-baseline gap-2 text-[13px]"
              title={f.evidence.join('\n')}
            >
              <span className="shrink-0">
                {f.generic ? t(`scan.generic.${f.label}` as 'scan.generic.readme') : f.label}
                {f.count > 1 && <span className="ml-1 text-faint">{f.count}</span>}
              </span>
              <span className="min-w-0 truncate font-mono text-[11px] text-faint">{f.evidence[0]}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// ── agent map ───────────────────────────────────────────────────────────────

function MapSection({ project, claudeInstalled }: { project: Project; claudeInstalled: boolean }) {
  const trpc = useTRPC();
  const { t, i18n } = useTranslation();
  const state = useQuery(trpc.map.get.queryOptions({ projectId: project.id }));
  const start = useMutation(trpc.map.start.mutationOptions());
  const cancel = useMutation(trpc.map.cancel.mutationOptions());

  const map = state.data?.map ?? null;
  const run = state.data?.run ?? null;
  const running = run?.status === 'running';
  const language = i18n.language === 'en' ? 'en' : 'ru';

  const action = !running && (
    <button
      type="button"
      disabled={!claudeInstalled || start.isPending}
      onClick={() => start.mutate({ projectId: project.id, language })}
      className={
        map
          ? 'rounded-md border border-line px-2.5 py-1 text-[12px] hover:border-accent hover:text-accent-fg disabled:opacity-50'
          : 'rounded-md border border-accent px-3 py-1.5 text-[13px] font-medium text-accent-fg hover:bg-accent/10 disabled:opacity-50'
      }
    >
      {map ? t('map.refresh') : t('map.start')}
    </button>
  );

  return (
    <Section title={t('map.title')} lead={map ? undefined : t('map.lead')} action={map ? action : undefined}>
      {!claudeInstalled && <Notice tone="warn">{t('map.claudeMissing')}</Notice>}

      {running && run && <RunProgress run={run} onCancel={() => cancel.mutate({ projectId: project.id })} />}
      {!running && run?.status === 'failed' && (
        <Notice tone="error">
          <strong className="font-medium">{t('map.failed')}.</strong> {run.error}
        </Notice>
      )}
      {!running && run?.status === 'cancelled' && <Notice>{t('map.cancelled')}</Notice>}
      {!running && run?.status === 'interrupted' && <Notice>{t('map.interrupted')}</Notice>}

      {map ? (
        <MapView map={map} costUsd={run?.status === 'done' ? run.costUsd : null} />
      ) : (
        !running && <div className="mt-1">{action}</div>
      )}
    </Section>
  );
}

function RunProgress({ run, onCancel }: { run: Run; onCancel: () => void }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const events = useQuery(trpc.runs.events.queryOptions({ runId: run.id }));
  const elapsed = useElapsed(run.startedAt);
  const shown = (events.data ?? [])
    .filter((e) => e.event.type === 'tool_call' || e.event.type === 'error')
    .slice(-12);

  return (
    <div className="rounded-lg border border-line bg-panel p-3" aria-live="polite">
      <div className="flex items-center gap-2">
        <span className="h-2 w-2 animate-pulse rounded-full bg-accent motion-reduce:animate-none" />
        <span className="text-[13px] font-medium">{t('map.running')}</span>
        <span className="text-[12px] text-faint tabular-nums">{elapsed}</span>
        <button
          type="button"
          onClick={onCancel}
          className="ml-auto rounded-md px-2 py-0.5 text-[12px] text-muted hover:bg-sel hover:text-ink"
        >
          {t('map.cancel')}
        </button>
      </div>
      {shown.length > 0 && (
        <ol className="mt-2 space-y-0.5 font-mono text-[11.5px] text-muted">
          {shown.map((e) => (
            <EventLine key={e.seq} event={e} />
          ))}
        </ol>
      )}
    </div>
  );
}

function EventLine({ event }: { event: AgentEvent }) {
  const { t } = useTranslation();
  const e = event.event;
  if (e.type === 'tool_call') {
    return (
      <li className="truncate">
        <span className="text-faint">{t(`log.${e.kind}`)}</span> {e.target}
      </li>
    );
  }
  if (e.type === 'error') return <li className="truncate text-modify">{t('log.retry')}</li>;
  return null;
}

function useElapsed(since: number): string {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const s = Math.max(0, Math.floor((now - since) / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

const kindOrder: ComponentKind[] = [
  'frontend',
  'backend',
  'service',
  'db',
  'library',
  'infra',
  'test',
  'other',
];

const kindColor: Record<ComponentKind, string> = {
  frontend: 'var(--hl-blue)',
  backend: 'var(--hl-purple)',
  service: 'var(--hl-teal)',
  db: 'var(--hl-amber)',
  library: 'var(--hl-slate)',
  infra: 'var(--hl-pink)',
  test: 'var(--hl-green)',
  other: 'var(--faint)',
};

function MapView({ map, costUsd }: { map: RepoMap; costUsd: number | null }) {
  const { t, i18n } = useTranslation();
  const json = map.json;
  const byId = new Map(json.components.map((c) => [c.id, c]));
  const date = new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium', timeStyle: 'short' }).format(
    map.createdAt,
  );

  return (
    <div>
      <p className="max-w-3xl text-[14px] leading-relaxed select-text">{json.summary}</p>
      <p className="mt-1 text-[11px] text-faint">
        {t('map.meta', { date, sha: map.commitSha?.slice(0, 7) ?? '—' })}
        {costUsd !== null &&
          t('map.cost', {
            cost: new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'USD' }).format(
              costUsd,
            ),
          })}
      </p>

      {kindOrder.map((kind) => {
        const components = json.components.filter((c) => c.kind === kind);
        if (components.length === 0) return null;
        return (
          <section key={kind} className="mt-6">
            <h3 className="flex items-center gap-2 text-[12px] font-medium text-muted">
              <span className="h-2 w-2 rounded-sm" style={{ background: kindColor[kind] }} />
              {t(`map.kinds.${kind}`)}
              <span className="text-faint">{components.length}</span>
            </h3>
            <div className="mt-2 grid grid-cols-2 gap-3">
              {components.map((c) => (
                <ComponentBlock key={c.id} component={c} map={map} byId={byId} />
              ))}
            </div>
          </section>
        );
      })}

      {(json.entrypoints.length > 0 || json.testCommandGuess) && (
        <dl className="mt-6 grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 border-t border-line pt-3 text-[12px]">
          {json.entrypoints.length > 0 && (
            <>
              <dt className="text-muted">{t('map.entrypoints')}</dt>
              <dd className="font-mono text-[11.5px] select-text">{json.entrypoints.join(', ')}</dd>
            </>
          )}
          {json.testCommandGuess && (
            <>
              <dt className="text-muted">{t('map.testCommand')}</dt>
              <dd className="font-mono text-[11.5px] select-text">{json.testCommandGuess}</dd>
            </>
          )}
        </dl>
      )}
    </div>
  );
}

function ComponentBlock(props: {
  component: RepoMapComponent;
  map: RepoMap;
  byId: Map<string, RepoMapComponent>;
}) {
  const { t } = useTranslation();
  const c = props.component;
  const name = (id: string) => props.byId.get(id)?.name ?? id;
  const outgoing = props.map.json.edges.filter((e) => e.from === c.id);
  const incoming = props.map.json.edges.filter((e) => e.to === c.id);
  const entitiesLabel =
    c.kind === 'db'
      ? t('map.entities.db')
      : c.kind === 'backend' || c.kind === 'service'
        ? t('map.entities.api')
        : t('map.entities.other');

  return (
    <article className="rounded-lg border border-line bg-raised p-3 select-text">
      <div className="flex items-baseline justify-between gap-2">
        <h4 className="text-[13.5px] font-semibold">{c.name}</h4>
        <span className="font-mono text-[11px] text-faint">{c.id}</span>
      </div>
      <p className="mt-1 text-[12.5px] leading-snug text-muted">{c.description}</p>

      {c.tech.length > 0 && (
        <ul className="mt-2 flex flex-wrap gap-1">
          {c.tech.map((tech) => (
            <li key={tech} className="rounded bg-panel px-1.5 py-px text-[11px] text-muted">
              {tech}
            </li>
          ))}
        </ul>
      )}

      {c.entities.length > 0 && (
        <Detail label={entitiesLabel}>
          <span className="font-mono text-[11.5px]">{c.entities.join(', ')}</span>
        </Detail>
      )}
      {c.paths.length > 0 && (
        <Detail label={t('map.paths')}>
          <span className="font-mono text-[11.5px] text-muted">{c.paths.join(', ')}</span>
        </Detail>
      )}
      {(outgoing.length > 0 || incoming.length > 0) && (
        <Detail label={t('map.links')}>
          <ul className="space-y-0.5">
            {outgoing.map((e) => (
              <li key={`o-${e.to}-${e.kind}`}>
                {t(`map.edgeKinds.${e.kind}`)} <strong className="font-medium">{name(e.to)}</strong>
                {e.note && <span className="text-faint"> — {e.note}</span>}
              </li>
            ))}
            {incoming.map((e) => (
              <li key={`i-${e.from}-${e.kind}`} className="text-muted">
                {t('map.incoming', { from: name(e.from), verb: t(`map.incomingVerbs.${e.kind}`) })}
              </li>
            ))}
          </ul>
        </Detail>
      )}
    </article>
  );
}

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="mt-2 text-[12px]">
      <div className="text-[11px] text-faint">{label}</div>
      <div className="mt-0.5">{children}</div>
    </div>
  );
}

// ── shared bits ─────────────────────────────────────────────────────────────

function Section(props: { title: string; lead?: string; action?: ReactNode; children: ReactNode }) {
  return (
    <section className="mt-8">
      <div className="flex items-baseline justify-between gap-4 border-b border-line pb-2">
        <div>
          <h2 className="text-[15px] font-semibold tracking-tight">{props.title}</h2>
          {props.lead && <p className="mt-0.5 text-[12.5px] text-muted">{props.lead}</p>}
        </div>
        {props.action}
      </div>
      <div className="mt-4">{props.children}</div>
    </section>
  );
}

function Notice(props: { tone?: 'warn' | 'error'; children: ReactNode }) {
  const tone =
    props.tone === 'error'
      ? 'border-remove/40 text-ink'
      : props.tone === 'warn'
        ? 'border-modify/50 text-ink'
        : 'border-line text-muted';
  return (
    <div className={`mb-3 rounded-lg border bg-panel px-3 py-2 text-[12.5px] select-text ${tone}`}>
      {props.children}
    </div>
  );
}
