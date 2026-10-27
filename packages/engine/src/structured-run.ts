import { AgentCancelledError, runClaudeStructured } from '@studio/agents';
import type { Db } from '@studio/db';
import { validationFollowUp } from '@studio/prompts';
import type { NormalizedEvent, Run, StudioEvent } from '@studio/schema';
import { z } from 'zod';

export interface AgentDeps {
  db: Db;
  notify: (event: StudioEvent) => void;
  /** The agent call; tests replace it with a fake. */
  runAgent?: typeof runClaudeStructured;
}

/** Validation retries after the first answer (SPEC §3.2). */
const MAX_RETRIES = 2;

/**
 * Runs a read-only agent until its structured answer passes the schema and `problems`, sending
 * validation errors back up to twice (SPEC §3.2). Records events, cost and the final run status;
 * `save` stores the valid answer before the run is marked done.
 */
export async function executeStructuredRun<T>(
  deps: AgentDeps,
  args: {
    run: Run;
    cwd: string;
    /** Builds the first prompt; a failure here fails the run like any other. */
    prompt: () => Promise<string>;
    schema: z.ZodType<T>;
    problems?: (value: T) => string[];
    model?: string;
    signal: AbortSignal;
    save: (value: T) => Promise<void>;
  },
): Promise<void> {
  const { db, notify } = deps;
  const { run } = args;
  const runAgent = deps.runAgent ?? runClaudeStructured;
  const changed = () => notify({ type: 'run', projectId: run.projectId, runId: run.id });
  const jsonSchema = z.toJSONSchema(args.schema);

  // Events are appended in order; the UI is told after each one lands.
  let pending = Promise.resolve();
  const record = (event: NormalizedEvent) => {
    pending = pending.then(() => db.appendEvent(run.id, event)).then(changed);
  };

  let costUsd = 0;
  try {
    let prompt = await args.prompt();
    let sessionId: string | undefined;
    for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
      const result = await runAgent({
        cwd: args.cwd,
        prompt,
        jsonSchema,
        model: args.model,
        resumeSessionId: sessionId,
        onEvent: record,
        signal: args.signal,
      });
      costUsd += result.costUsd;

      const parsed = args.schema.safeParse(result.output);
      const problems = parsed.success
        ? (args.problems?.(parsed.data) ?? [])
        : parsed.error.issues.map((i) => `${i.path.join('.') || 'answer'}: ${i.message}`);
      if (parsed.success && problems.length === 0) {
        await pending;
        await args.save(parsed.data);
        await db.finishRun(run.id, { status: 'done', costUsd });
        return;
      }
      if (attempt === MAX_RETRIES || !result.sessionId) {
        throw new Error(`The agent's answer did not pass validation: ${problems.join('; ')}`);
      }
      record({ type: 'error', message: problems.join('; ') });
      prompt = validationFollowUp(problems);
      sessionId = result.sessionId;
    }
  } catch (error) {
    await pending;
    if (error instanceof AgentCancelledError || args.signal.aborted) {
      await db.finishRun(run.id, { status: 'cancelled', costUsd });
    } else {
      const message = error instanceof Error ? error.message : String(error);
      await db.appendEvent(run.id, { type: 'error', message });
      await db.finishRun(run.id, { status: 'failed', error: message, costUsd });
    }
  }
}

/** One background run per key (a project, a request); a second start returns the active run. */
export class RunSlots {
  private readonly running = new Map<string, { run: Run; abort: AbortController }>();

  constructor(private readonly notify: (event: StudioEvent) => void) {}

  active(key: string): Run | undefined {
    return this.running.get(key)?.run;
  }

  async start(
    key: string,
    create: () => Promise<Run>,
    execute: (run: Run, signal: AbortSignal) => Promise<void>,
  ) {
    const active = this.running.get(key);
    if (active) return active.run;

    const run = await create();
    const abort = new AbortController();
    this.running.set(key, { run, abort });
    const changed = () => this.notify({ type: 'run', projectId: run.projectId, runId: run.id });
    changed();
    void execute(run, abort.signal).finally(() => {
      this.running.delete(key);
      changed();
    });
    return run;
  }

  cancel(key: string): void {
    this.running.get(key)?.abort.abort();
  }
}
