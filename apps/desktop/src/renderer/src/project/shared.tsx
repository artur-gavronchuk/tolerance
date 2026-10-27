import type { AgentEvent, Run } from '@studio/schema';
import { useQuery } from '@tanstack/react-query';
import { type ReactNode, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTRPC } from '../lib/trpc';

/** A running agent: what it is doing right now, how long, and a stop button. */
export function RunProgress({ run, label, onCancel }: { run: Run; label: string; onCancel: () => void }) {
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
        <span className="text-[13px] font-medium">{label}</span>
        <span className="text-[12px] text-faint tabular-nums">{elapsed}</span>
        <button
          type="button"
          onClick={onCancel}
          className="ml-auto rounded-md px-2 py-0.5 text-[12px] text-muted hover:bg-sel hover:text-ink"
        >
          {t('run.cancel')}
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

export function useElapsed(since: number): string {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const s = Math.max(0, Math.floor((now - since) / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

// ── shared bits ─────────────────────────────────────────────────────────────

export function Section(props: { title: string; lead?: string; action?: ReactNode; children: ReactNode }) {
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

export function Notice(props: { tone?: 'warn' | 'error'; children: ReactNode }) {
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
