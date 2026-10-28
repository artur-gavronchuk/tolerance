import type { Run, Task, TaskSpecJson } from '@studio/schema';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { DiffText } from '../diff/DiffText';
import { useTRPC } from '../lib/trpc';
import { Notice, RunProgress } from './shared';

export interface TaskState {
  task: Task;
  run: Run | null;
}

/**
 * Under an approved plan: each task of the chosen variant with a button that runs it in its own
 * worktree, the agent's progress, and then what it changed. One task at a time, by hand.
 */
export function ExecutionBlock(props: {
  planId: string;
  tasks: TaskSpecJson[];
  states: TaskState[];
  language: 'ru' | 'en';
  claudeInstalled: boolean;
}) {
  const { t } = useTranslation();
  const busyKey = props.states.find((s) => s.run?.status === 'running')?.task.key ?? null;

  return (
    <section className="mt-4">
      <h5 className="text-[13px] font-semibold">{t('exec.title')}</h5>
      <p className="mt-0.5 text-[12px] text-muted">{t('exec.lead')}</p>
      <ol className="mt-2 space-y-2">
        {props.tasks.map((spec) => (
          <li key={spec.key}>
            <TaskExecution
              planId={props.planId}
              spec={spec}
              state={props.states.find((s) => s.task.key === spec.key) ?? null}
              blockedBy={busyKey && busyKey !== spec.key ? busyKey : null}
              language={props.language}
              claudeInstalled={props.claudeInstalled}
            />
          </li>
        ))}
      </ol>
    </section>
  );
}

function TaskExecution(props: {
  planId: string;
  spec: TaskSpecJson;
  state: TaskState | null;
  blockedBy: string | null;
  language: 'ru' | 'en';
  claudeInstalled: boolean;
}) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const start = useMutation(trpc.tasks.run.mutationOptions());
  const cancel = useMutation(trpc.tasks.cancel.mutationOptions());
  const { spec, state } = props;
  const run = state?.run ?? null;
  const running = run?.status === 'running';
  const status = run?.status ?? 'idle';

  return (
    <article className="rounded-md border border-line bg-raised" data-task={spec.key}>
      <header className="flex items-center gap-2 px-3 py-2 text-[12.5px]">
        <span className="font-mono text-[11px] text-faint">{spec.key}</span>
        <span className="min-w-0 flex-1 truncate">{spec.title}</span>
        <span className="shrink-0 text-[11.5px] text-muted">{t(`exec.status.${status}`)}</span>
        {!running && (
          <button
            type="button"
            disabled={!props.claudeInstalled || !!props.blockedBy || start.isPending}
            title={props.blockedBy ? t('exec.busy') : undefined}
            onClick={() =>
              start.mutate({ planId: props.planId, taskKey: spec.key, language: props.language })
            }
            className="shrink-0 rounded-md border border-accent px-2.5 py-0.5 text-[12px] font-medium text-accent-fg hover:bg-accent/10 disabled:opacity-50"
          >
            {run ? t('exec.rerun') : t('exec.run')}
          </button>
        )}
      </header>

      {(start.error || running || state) && (
        <div className="border-t border-line px-3 py-2">
          {start.error && (
            <Notice tone="error">{t('errors.generic', { message: start.error.message })}</Notice>
          )}
          {running && run && (
            <RunProgress
              run={run}
              label={t('exec.running', { key: spec.key })}
              onCancel={() => cancel.mutate({ planId: props.planId })}
            />
          )}
          {!running && run?.status === 'failed' && (
            <Notice tone="error">
              <strong className="font-medium">{t('exec.failed')}.</strong> {run.error}
            </Notice>
          )}
          {!running && run?.status === 'cancelled' && <Notice>{t('exec.cancelled')}</Notice>}
          {!running && run?.status === 'interrupted' && <Notice>{t('exec.interrupted')}</Notice>}
          {!running && state && <TaskResult task={state.task} run={run} />}
        </div>
      )}
    </article>
  );
}

/** What a finished task left: the agent's summary, where the work is, changed files and the diff. */
function TaskResult({ task, run }: { task: Task; run: Run | null }) {
  const trpc = useTRPC();
  const { t, i18n } = useTranslation();
  const diff = useQuery(trpc.tasks.diff.queryOptions({ taskId: task.id }));
  const events = useQuery({ ...trpc.runs.events.queryOptions({ runId: run?.id ?? '' }), enabled: !!run });
  const remove = useMutation(trpc.tasks.removeWorktree.mutationOptions());
  const summary = [...(events.data ?? [])].reverse().find((e) => e.event.type === 'assistant_message')?.event;
  const cost =
    run?.status === 'done' && run.costUsd !== null
      ? new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'USD' }).format(run.costUsd)
      : null;

  return (
    <div className="space-y-2 text-[12px] select-text">
      {summary?.type === 'assistant_message' && (
        <div>
          <p className="text-[11px] font-medium text-faint">{t('exec.agentSays')}</p>
          <p className="mt-0.5 leading-relaxed whitespace-pre-wrap">{summary.text}</p>
        </div>
      )}

      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-[11.5px]">
        <dt className="text-faint">{t('exec.branch')}</dt>
        <dd className="min-w-0 truncate font-mono">{task.branch}</dd>
        <dt className="text-faint">{t('exec.worktree')}</dt>
        <dd className="min-w-0 truncate font-mono" title={task.worktreePath ?? undefined}>
          {task.worktreePath ?? <span className="font-sans text-muted">{t('exec.worktreeRemoved')}</span>}
        </dd>
      </dl>

      {diff.error && <Notice tone="error">{t('errors.generic', { message: diff.error.message })}</Notice>}
      {diff.data && diff.data.files.length === 0 && <p className="text-muted">{t('exec.noChanges')}</p>}
      {diff.data && diff.data.files.length > 0 && (
        <>
          <div>
            <p className="text-[11px] font-medium text-faint">
              {t('exec.files')} ({diff.data.files.length})
            </p>
            <ul className="mt-0.5 space-y-0.5 font-mono text-[11.5px]">
              {diff.data.files.map((f) => (
                <li key={f.path} className="flex gap-2">
                  <span className="w-16 shrink-0 font-sans text-[11px] text-muted">
                    {t(`exec.fileStatus.${f.status}`)}
                  </span>
                  <span className="min-w-0 flex-1 truncate">{f.path}</span>
                  {!!f.additions && <span className="text-add">+{f.additions}</span>}
                  {!!f.deletions && <span className="text-remove">−{f.deletions}</span>}
                </li>
              ))}
            </ul>
          </div>
          <DiffText patch={diff.data.patch} />
          {diff.data.truncated && <p className="text-modify">{t('exec.truncated')}</p>}
        </>
      )}

      <div className="flex items-center justify-between gap-3 pt-1">
        <span className="text-[11px] text-faint">{cost && t('exec.cost', { cost })}</span>
        <span className="flex items-center gap-3">
          {remove.error && (
            <span className="text-remove">{t('errors.generic', { message: remove.error.message })}</span>
          )}
          {task.worktreePath && (
            <button
              type="button"
              disabled={remove.isPending}
              onClick={() => remove.mutate({ taskId: task.id })}
              className="rounded-md border border-line px-2.5 py-0.5 text-[12px] hover:border-remove hover:text-remove disabled:opacity-50"
            >
              {t('exec.removeWorktree')}
            </button>
          )}
        </span>
      </div>
    </div>
  );
}
