import type { ComponentChange, PlanVariant, PlanWithVariants, Run, TaskSpecJson } from '@studio/schema';
import { useMutation } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useTRPC } from '../lib/trpc';
import { Notice, RunProgress } from './shared';

const changeColor: Record<ComponentChange, string> = {
  add: 'var(--add)',
  modify: 'var(--modify)',
  remove: 'var(--remove)',
};

/** Under a task: two plan variants side by side, and the one the developer approves (SPEC §3.6, Plan). */
export function PlanBlock(props: {
  requestId: string;
  plan: PlanWithVariants | null;
  run: Run | null;
  language: 'ru' | 'en';
  claudeInstalled: boolean;
}) {
  const trpc = useTRPC();
  const { t, i18n } = useTranslation();
  const start = useMutation(trpc.plans.start.mutationOptions());
  const cancel = useMutation(trpc.plans.cancel.mutationOptions());
  const reopen = useMutation(trpc.plans.reopen.mutationOptions());
  const { plan, run } = props;
  const running = run?.status === 'running';
  const approved = plan?.status === 'approved';
  const error = start.error ?? reopen.error;
  const cost =
    plan && run?.status === 'done' && run.id === plan.runId && run.costUsd !== null
      ? new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'USD' }).format(run.costUsd)
      : null;

  return (
    <section className="mt-4 border-t border-line pt-3">
      <div className="flex items-baseline justify-between gap-4">
        <div>
          <h4 className="text-[13px] font-semibold">{t('plan.title')}</h4>
          {!plan && !running && <p className="mt-0.5 text-[12px] text-muted">{t('plan.lead')}</p>}
        </div>
        {!running && !approved && (
          <button
            type="button"
            disabled={!props.claudeInstalled || start.isPending}
            onClick={() => start.mutate({ requestId: props.requestId, language: props.language })}
            className="shrink-0 rounded-md border border-accent px-2.5 py-1 text-[12px] font-medium text-accent-fg hover:bg-accent/10 disabled:opacity-50"
          >
            {plan ? t('plan.remake') : t('plan.make')}
          </button>
        )}
        {approved && plan && (
          <button
            type="button"
            disabled={reopen.isPending}
            onClick={() => reopen.mutate({ planId: plan.id })}
            className="shrink-0 rounded-md border border-line px-2.5 py-1 text-[12px] hover:border-accent hover:text-accent-fg disabled:opacity-50"
          >
            {t('plan.reopen')}
          </button>
        )}
      </div>

      <div className="mt-3">
        {error && <Notice tone="error">{t('errors.generic', { message: error.message })}</Notice>}
        {running && run && (
          <RunProgress
            run={run}
            label={t('plan.running')}
            onCancel={() => cancel.mutate({ requestId: props.requestId })}
          />
        )}
        {!running && run?.status === 'failed' && (
          <Notice tone="error">
            <strong className="font-medium">{t('plan.failed')}.</strong> {run.error}
          </Notice>
        )}
        {!running && run?.status === 'cancelled' && <Notice>{t('plan.cancelled')}</Notice>}
        {!running && run?.status === 'interrupted' && <Notice>{t('plan.interrupted')}</Notice>}

        {plan && (
          <>
            <div className="grid grid-cols-2 gap-3">
              {plan.variants.map((v) => (
                <VariantCard
                  key={v.id}
                  planId={plan.id}
                  variant={v}
                  chosen={plan.chosenVariantId === v.id}
                  approved={approved}
                />
              ))}
            </div>
            {approved && <p className="mt-2 text-[11.5px] text-faint">{t('plan.approvedHint')}</p>}
            {cost && <p className="mt-2 text-[11px] text-faint">{t('plan.cost', { cost })}</p>}
          </>
        )}
      </div>
    </section>
  );
}

function VariantCard(props: { planId: string; variant: PlanVariant; chosen: boolean; approved: boolean }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const approve = useMutation(trpc.plans.approve.mutationOptions());
  const v = props.variant.json;
  const dimmed = props.approved && !props.chosen;

  return (
    <article
      className={`flex min-w-0 flex-col rounded-lg border bg-panel p-3 select-text ${
        props.chosen ? 'border-accent' : 'border-line'
      } ${dimmed ? 'opacity-60' : ''}`}
    >
      <header className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="text-[11px] text-faint">{t('plan.variant', { n: props.variant.position + 1 })}</p>
          <h5 className="text-[13.5px] font-semibold leading-snug">{v.title}</h5>
        </div>
        <span className="shrink-0 text-[12px]">
          <span className="text-muted">{t('tasks.complexity')}</span>{' '}
          <strong className="font-semibold">{v.complexity}</strong>
        </span>
      </header>

      <p className="mt-2 text-[12.5px] leading-relaxed">{v.approach}</p>

      <Lists
        items={[
          [t('plan.pros'), v.tradeoffs.pros],
          [t('plan.cons'), v.tradeoffs.cons],
          [t('plan.risks'), v.tradeoffs.risks],
        ]}
      />

      {v.affectedComponents.length > 0 && (
        <Labeled label={t('plan.components')}>
          <ul className="flex flex-wrap gap-1.5">
            {v.affectedComponents.map((c) => (
              <li
                key={c.componentId}
                title={c.note || undefined}
                className="rounded border px-1.5 py-0.5 font-mono text-[11px]"
                style={{ borderColor: changeColor[c.change], color: changeColor[c.change] }}
              >
                {c.componentId} · {t(`plan.change.${c.change}`)}
              </li>
            ))}
          </ul>
        </Labeled>
      )}

      <Labeled label={t('plan.tasks', { count: v.tasks.length })}>
        <ol className="space-y-1">
          {v.tasks.map((task) => (
            <li key={task.key}>
              <TaskRow task={task} />
            </li>
          ))}
        </ol>
      </Labeled>

      <div className="mt-auto flex items-center justify-end gap-3 pt-3">
        {approve.error && (
          <span className="min-w-0 truncate text-[11.5px] text-remove" title={approve.error.message}>
            {t('errors.generic', { message: approve.error.message })}
          </span>
        )}
        {props.chosen && props.approved ? (
          <span className="rounded-md border border-accent px-2 py-0.5 text-[12px] font-medium text-accent-fg">
            ✓ {t('plan.approved')}
          </span>
        ) : (
          !props.approved && (
            <button
              type="button"
              disabled={approve.isPending}
              onClick={() => approve.mutate({ planId: props.planId, variantId: props.variant.id })}
              className="rounded-md border border-accent px-2.5 py-1 text-[12px] font-medium text-accent-fg hover:bg-accent/10 disabled:opacity-50"
            >
              {t('plan.approve')}
            </button>
          )
        )}
      </div>
    </article>
  );
}

/** One task of a variant: a summary line that opens to its description, scope and criteria. */
function TaskRow({ task }: { task: TaskSpecJson }) {
  const { t } = useTranslation();
  return (
    <details className="group rounded-md border border-line bg-raised">
      <summary className="flex cursor-default items-baseline gap-2 px-2 py-1 text-[12.5px]">
        <span className="font-mono text-[11px] text-faint">{task.key}</span>
        <span className="min-w-0 flex-1">{task.title}</span>
        <span className="shrink-0 text-[11px] text-faint">
          {task.dependsOn.length > 0
            ? t('plan.after', { keys: task.dependsOn.join(', ') })
            : t('plan.parallel')}
        </span>
      </summary>
      <div className="space-y-2 border-t border-line px-2 py-2 text-[12px]">
        <p className="leading-relaxed text-muted">{task.description}</p>
        <Globs label={t('plan.scope')} globs={task.scope.allow} />
        <Globs label={t('plan.deny')} globs={task.scope.deny} />
        <div>
          <p className="text-[11px] text-faint">{t('plan.acceptance')}</p>
          <ul className="mt-0.5 space-y-0.5">
            {task.acceptance.map((c) => (
              <li key={c.id}>
                <span className="font-mono text-[11px] text-faint">{c.id}</span>{' '}
                <span className="rounded bg-panel px-1 text-[10.5px] text-muted">
                  {t(`plan.check.${c.check}`)}
                </span>{' '}
                {c.text}
                {c.command && (
                  <code className="mt-0.5 block font-mono text-[11px] text-muted">{c.command}</code>
                )}
              </li>
            ))}
          </ul>
        </div>
      </div>
    </details>
  );
}

function Globs({ label, globs }: { label: string; globs: string[] }) {
  if (globs.length === 0) return null;
  return (
    <p>
      <span className="text-[11px] text-faint">{label}:</span>{' '}
      <span className="font-mono text-[11px]">{globs.join(', ')}</span>
    </p>
  );
}

function Lists({ items }: { items: [string, string[]][] }) {
  return (
    <>
      {items
        .filter(([, list]) => list.length > 0)
        .map(([label, list]) => (
          <Labeled key={label} label={label}>
            <ul className="list-disc space-y-0.5 pl-4 text-[12px] marker:text-faint">
              {list.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
          </Labeled>
        ))}
    </>
  );
}

function Labeled({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="mt-2.5">
      <h6 className="text-[11px] font-medium text-faint">{label}</h6>
      <div className="mt-1">{children}</div>
    </section>
  );
}
