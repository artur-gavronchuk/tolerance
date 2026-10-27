import type {
  Assessment,
  AssessmentVerdict,
  ChangeRequest,
  PlanWithVariants,
  Project,
  Run,
} from '@studio/schema';
import { useMutation, useQuery } from '@tanstack/react-query';
import { type FormEvent, type ReactNode, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useTRPC } from '../lib/trpc';
import type { TaskState } from './ExecutionBlock';
import { PlanBlock } from './PlanBlock';
import { Notice, RunProgress, Section } from './shared';

/** The first thing to do in a project: write a task and get the agent's opinion of it. */
export function TasksSection({ project, claudeInstalled }: { project: Project; claudeInstalled: boolean }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const list = useQuery(trpc.requests.list.queryOptions({ projectId: project.id }));
  const items = list.data ?? [];

  return (
    <Section title={t('tasks.title')} lead={items.length === 0 ? t('tasks.lead') : undefined}>
      {!claudeInstalled && <Notice tone="warn">{t('map.claudeMissing')}</Notice>}
      <Composer
        project={project}
        disabled={!claudeInstalled}
        autoFocus={list.isSuccess && items.length === 0}
      />
      {items.length > 0 && (
        <ol className="mt-6 space-y-4">
          {items.map((item) => (
            <li key={item.request.id}>
              <TaskCard {...item} claudeInstalled={claudeInstalled} />
            </li>
          ))}
        </ol>
      )}
    </Section>
  );
}

function useLanguage(): 'ru' | 'en' {
  const { i18n } = useTranslation();
  return i18n.language === 'en' ? 'en' : 'ru';
}

function Composer(props: { project: Project; disabled: boolean; autoFocus: boolean }) {
  const trpc = useTRPC();
  const { t } = useTranslation();
  const language = useLanguage();
  const [text, setText] = useState('');
  const create = useMutation(trpc.requests.create.mutationOptions());
  const input = useRef<HTMLTextAreaElement>(null);
  // Focused once the project turns out to have no tasks yet: writing one is the first thing to do.
  useEffect(() => {
    if (props.autoFocus) input.current?.focus();
  }, [props.autoFocus]);
  const canSend = !props.disabled && !create.isPending && text.trim().length > 0;

  function submit(e?: FormEvent) {
    e?.preventDefault();
    if (!canSend) return;
    create.mutate(
      { projectId: props.project.id, text: text.trim(), language },
      { onSuccess: () => setText('') },
    );
  }

  return (
    <form onSubmit={submit} className="rounded-lg border border-line bg-raised focus-within:border-accent">
      <label htmlFor="task-text" className="sr-only">
        {t('tasks.label')}
      </label>
      <textarea
        id="task-text"
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit();
        }}
        rows={4}
        ref={input}
        // The form's border shows focus; the global focus ring would draw a second frame inside it.
        style={{ outline: 'none' }}
        placeholder={t('tasks.placeholder')}
        className="block w-full resize-y bg-transparent px-3 pt-2.5 pb-1 text-[13.5px] leading-relaxed placeholder:text-faint"
      />
      <div className="flex items-center justify-between gap-3 px-3 pb-2.5">
        <span className="text-[11.5px] text-faint">
          {create.error ? (
            <span className="text-remove">{t('errors.generic', { message: create.error.message })}</span>
          ) : (
            t('tasks.hint')
          )}
        </span>
        <button
          type="submit"
          disabled={!canSend}
          className="shrink-0 rounded-md border border-accent px-3 py-1 text-[13px] font-medium text-accent-fg hover:bg-accent/10 disabled:opacity-50"
        >
          {t('tasks.submit')}
        </button>
      </div>
    </form>
  );
}

function TaskCard(props: {
  request: ChangeRequest;
  assessment: Assessment | null;
  run: Run | null;
  plan: PlanWithVariants | null;
  planRun: Run | null;
  tasks: TaskState[];
  claudeInstalled: boolean;
}) {
  const trpc = useTRPC();
  const { t, i18n } = useTranslation();
  const language = useLanguage();
  const assess = useMutation(trpc.requests.assess.mutationOptions());
  const cancel = useMutation(trpc.requests.cancel.mutationOptions());
  const remove = useMutation(trpc.requests.remove.mutationOptions());
  const { request, assessment, run } = props;
  const running = run?.status === 'running';
  const date = new Intl.DateTimeFormat(i18n.language, { dateStyle: 'medium', timeStyle: 'short' }).format(
    request.createdAt,
  );
  const reassess = () => assess.mutate({ requestId: request.id, language });

  return (
    <article className="rounded-lg border border-line bg-raised">
      <header className="flex items-start gap-4 border-b border-line px-4 py-3">
        <p className="min-w-0 flex-1 text-[14px] leading-relaxed whitespace-pre-wrap select-text">
          {request.text}
        </p>
        <div className="flex shrink-0 flex-col items-end gap-1">
          <span className="text-[11px] text-faint">{date}</span>
          <button
            type="button"
            onClick={() => remove.mutate({ requestId: request.id })}
            className="rounded px-1.5 py-0.5 text-[11.5px] text-muted hover:bg-panel hover:text-remove"
          >
            {t('tasks.remove')}
          </button>
        </div>
      </header>

      <div className="px-4 py-3">
        {running && run && (
          <RunProgress
            run={run}
            label={t('tasks.running')}
            onCancel={() => cancel.mutate({ requestId: request.id })}
          />
        )}
        {!running && run?.status === 'failed' && (
          <Notice tone="error">
            <strong className="font-medium">{t('tasks.failed')}.</strong> {run.error}
          </Notice>
        )}
        {!running && run?.status === 'cancelled' && <Notice>{t('tasks.cancelled')}</Notice>}
        {!running && run?.status === 'interrupted' && <Notice>{t('tasks.interrupted')}</Notice>}

        {assessment && !running && <AssessmentView assessment={assessment} run={run} />}

        {!running && (
          <div className="mt-3 flex justify-end">
            <button
              type="button"
              disabled={!props.claudeInstalled || assess.isPending}
              onClick={reassess}
              className="rounded-md border border-line px-2.5 py-1 text-[12px] hover:border-accent hover:text-accent-fg disabled:opacity-50"
            >
              {assessment ? t('tasks.reassess') : t('tasks.assess')}
            </button>
          </div>
        )}

        {/* Planning follows the assessment; a plan made earlier stays visible while the task is reassessed. */}
        {((assessment && !running) || props.plan || props.planRun) && (
          <PlanBlock
            requestId={request.id}
            plan={props.plan}
            run={props.planRun}
            tasks={props.tasks}
            language={language}
            claudeInstalled={props.claudeInstalled}
          />
        )}
      </div>
    </article>
  );
}

const verdictColor: Record<AssessmentVerdict, string> = {
  go: 'var(--add)',
  clarify: 'var(--modify)',
  reconsider: 'var(--remove)',
};

function AssessmentView({ assessment, run }: { assessment: Assessment; run: Run | null }) {
  const { t, i18n } = useTranslation();
  const a = assessment.json;
  const cost =
    run?.status === 'done' && run.costUsd !== null
      ? new Intl.NumberFormat(i18n.language, { style: 'currency', currency: 'USD' }).format(run.costUsd)
      : null;

  return (
    <div className="select-text">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
        <span
          className="inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-[12.5px] font-medium"
          style={{ borderColor: verdictColor[a.verdict], color: verdictColor[a.verdict] }}
        >
          <span className="h-1.5 w-1.5 rounded-full" style={{ background: verdictColor[a.verdict] }} />
          {t(`tasks.verdict.${a.verdict}`)}
        </span>
        <span className="text-[12.5px]">
          <span className="text-muted">{t('tasks.complexity')}</span>{' '}
          <strong className="font-semibold">{a.complexity}</strong>
          <span className="text-muted"> — {a.complexityReason}</span>
        </span>
      </div>

      <Block label={t('tasks.understanding')}>
        <p className="text-[13px] leading-relaxed text-muted">{a.understanding}</p>
      </Block>
      <Block label={t('tasks.opinion')}>
        <p className="max-w-3xl text-[13.5px] leading-relaxed">{a.opinion}</p>
      </Block>

      <div className="grid grid-cols-2 gap-x-6">
        {a.affected.length > 0 && (
          <Block label={t('tasks.affected')}>
            <ul className="space-y-0.5 text-[12.5px]">
              {a.affected.map((f) => (
                <li key={f.path} className="min-w-0">
                  <span className="font-mono text-[11.5px]">{f.path}</span>
                  {f.note && <span className="text-muted"> — {f.note}</span>}
                </li>
              ))}
            </ul>
          </Block>
        )}
        {a.approach.length > 0 && (
          <Block label={t('tasks.approach')}>
            <ol className="list-decimal space-y-0.5 pl-4 text-[12.5px] marker:text-faint">
              {a.approach.map((step) => (
                <li key={step}>{step}</li>
              ))}
            </ol>
          </Block>
        )}
        {a.risks.length > 0 && (
          <Block label={t('tasks.risks')}>
            <BulletList items={a.risks} />
          </Block>
        )}
        {a.questions.length > 0 && (
          <Block label={t('tasks.questions')}>
            <BulletList items={a.questions} />
          </Block>
        )}
      </div>

      {cost && <p className="mt-3 text-[11px] text-faint">{t('tasks.cost', { cost })}</p>}
    </div>
  );
}

function Block({ label, children }: { label: string; children: ReactNode }) {
  return (
    <section className="mt-3">
      <h4 className="text-[11.5px] font-medium text-faint">{label}</h4>
      <div className="mt-1">{children}</div>
    </section>
  );
}

function BulletList({ items }: { items: string[] }) {
  return (
    <ul className="list-disc space-y-0.5 pl-4 text-[12.5px] marker:text-faint">
      {items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  );
}
