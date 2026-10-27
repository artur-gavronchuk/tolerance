import { listFiles } from '@studio/git';
import { mapPrompt } from '@studio/prompts';
import { type Project, RepoMapJson, type RepoScan, type Run, repoMapProblems } from '@studio/schema';
import { scanRepo } from './scan';
import { type AgentDeps, executeStructuredRun, RunSlots } from './structured-run';

export type MapperDeps = AgentDeps;

/** Builds the repo map with an agent (SPEC §3.5, Mapper). One run per project at a time. */
export class Mapper {
  private readonly slots: RunSlots;

  constructor(private readonly deps: MapperDeps) {
    this.slots = new RunSlots(deps.notify);
  }

  activeRun(projectId: string): Run | undefined {
    return this.slots.active(projectId);
  }

  async start(project: Project, opts: { language: 'ru' | 'en'; model?: string }): Promise<Run> {
    return this.slots.start(
      project.id,
      () => this.deps.db.startRun({ projectId: project.id, kind: 'map', agentId: 'claude' }),
      async (run, signal) => {
        let commitSha: string | null = null;
        await executeStructuredRun(this.deps, {
          run,
          cwd: project.repoPath,
          prompt: async () => {
            const [scan, files] = await Promise.all([
              scanRepo(project.repoPath),
              listFiles(project.repoPath),
            ]);
            commitSha = scan.commitSha;
            return mapPrompt({
              scan: formatScan(scan),
              tree: formatTree(files),
              language: opts.language === 'ru' ? 'Russian' : 'English',
            });
          },
          schema: RepoMapJson,
          problems: repoMapProblems,
          model: opts.model,
          signal,
          save: async (json) => {
            await this.deps.db.addRepoMap({ projectId: project.id, commitSha, json });
            this.deps.notify({ type: 'map', projectId: project.id });
          },
        });
      },
    );
  }

  cancel(projectId: string): void {
    this.slots.cancel(projectId);
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
