import { AgentCancelledError, runClaudeStructured } from '@studio/agents';
import type { Db } from '@studio/db';
import { listFiles } from '@studio/git';
import { mapPrompt, validationFollowUp } from '@studio/prompts';
import {
  type Project,
  RepoMapJson,
  type RepoScan,
  type Run,
  repoMapProblems,
  type StudioEvent,
} from '@studio/schema';
import { z } from 'zod';
import { scanRepo } from './scan';

export interface MapperDeps {
  db: Db;
  notify: (event: StudioEvent) => void;
  /** The agent call; tests replace it with a fake. */
  runAgent?: typeof runClaudeStructured;
}

/** Validation retries after the first answer (SPEC §3.2). */
const MAX_RETRIES = 2;

const repoMapJsonSchema = z.toJSONSchema(RepoMapJson);

/** Builds the repo map with an agent (SPEC §3.5, Mapper). One run per project at a time. */
export class Mapper {
  private readonly running = new Map<string, { run: Run; abort: AbortController }>();

  constructor(private readonly deps: MapperDeps) {}

  activeRun(projectId: string): Run | undefined {
    return this.running.get(projectId)?.run;
  }

  async start(project: Project, opts: { language: 'ru' | 'en'; model?: string }): Promise<Run> {
    const active = this.running.get(project.id);
    if (active) return active.run;

    const run = await this.deps.db.startRun({ projectId: project.id, kind: 'map', agentId: 'claude' });
    const abort = new AbortController();
    this.running.set(project.id, { run, abort });
    this.deps.notify({ type: 'run', projectId: project.id, runId: run.id });

    void this.execute(project, run, opts, abort.signal).finally(() => {
      this.running.delete(project.id);
      this.deps.notify({ type: 'run', projectId: project.id, runId: run.id });
    });
    return run;
  }

  cancel(projectId: string): void {
    this.running.get(projectId)?.abort.abort();
  }

  private async execute(
    project: Project,
    run: Run,
    opts: { language: 'ru' | 'en'; model?: string },
    signal: AbortSignal,
  ): Promise<void> {
    const { db, notify } = this.deps;
    const runAgent = this.deps.runAgent ?? runClaudeStructured;

    // Events are appended in order; the UI is told after each one lands.
    let pending = Promise.resolve();
    const record = (event: Parameters<Db['appendEvent']>[1]) => {
      pending = pending
        .then(() => db.appendEvent(run.id, event))
        .then(() => notify({ type: 'run', projectId: project.id, runId: run.id }));
    };

    let costUsd = 0;
    try {
      const [scan, files] = await Promise.all([scanRepo(project.repoPath), listFiles(project.repoPath)]);
      let prompt = mapPrompt({
        scan: formatScan(scan),
        tree: formatTree(files),
        language: opts.language === 'ru' ? 'Russian' : 'English',
      });
      let sessionId: string | undefined;

      for (let attempt = 0; attempt <= MAX_RETRIES; attempt++) {
        const result = await runAgent({
          cwd: project.repoPath,
          prompt,
          jsonSchema: repoMapJsonSchema,
          model: opts.model,
          resumeSessionId: sessionId,
          onEvent: record,
          signal,
        });
        costUsd += result.costUsd;

        const parsed = RepoMapJson.safeParse(result.output);
        const problems = parsed.success
          ? repoMapProblems(parsed.data)
          : parsed.error.issues.map((i) => `${i.path.join('.') || 'answer'}: ${i.message}`);
        if (parsed.success && problems.length === 0) {
          await pending;
          await db.addRepoMap({ projectId: project.id, commitSha: scan.commitSha, json: parsed.data });
          await db.finishRun(run.id, { status: 'done', costUsd });
          notify({ type: 'map', projectId: project.id });
          return;
        }
        if (attempt === MAX_RETRIES || !result.sessionId) {
          throw new Error(`The agent's map did not pass validation: ${problems.join('; ')}`);
        }
        record({ type: 'error', message: problems.join('; ') });
        prompt = validationFollowUp(problems);
        sessionId = result.sessionId;
      }
    } catch (error) {
      await pending;
      if (error instanceof AgentCancelledError || signal.aborted) {
        await db.finishRun(run.id, { status: 'cancelled', costUsd });
      } else {
        const message = error instanceof Error ? error.message : String(error);
        await db.appendEvent(run.id, { type: 'error', message });
        await db.finishRun(run.id, { status: 'failed', error: message, costUsd });
      }
    }
  }
}

/** The quick scan as a short list for the prompt. */
export function formatScan(scan: RepoScan): string {
  const languages = scan.languages
    .filter((l) => l.share >= 0.01)
    .map((l) => `${l.name} ${Math.round(l.share * 100)}%`)
    .join(', ');
  const findings = scan.findings.map(
    (f) =>
      `- ${f.category}: ${f.label}${f.count > 1 ? ` (${f.count} files)` : ''} — ${f.evidence.slice(0, 4).join(', ')}`,
  );
  return [`Files: ${scan.totalFiles}. Languages: ${languages || 'unknown'}.`, ...findings].join('\n');
}

/** Directories up to two levels deep with how many files each holds, largest first within a level. */
export function formatTree(files: string[], maxLines = 120): string {
  const counts = new Map<string, number>();
  let rootFiles = 0;
  for (const file of files) {
    const parts = file.split('/');
    if (parts.length === 1) rootFiles++;
    for (let depth = 1; depth <= Math.min(2, parts.length - 1); depth++) {
      const dir = `${parts.slice(0, depth).join('/')}/`;
      counts.set(dir, (counts.get(dir) ?? 0) + 1);
    }
  }
  const lines = [...counts.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([dir, n]) => `${'  '.repeat(dir.split('/').length - 2)}${dir} ${n}`);
  const shown = lines.slice(0, maxLines);
  if (lines.length > maxLines) shown.push(`… ${lines.length - maxLines} more directories`);
  return [`./ ${rootFiles} files at the root`, ...shown].join('\n');
}
