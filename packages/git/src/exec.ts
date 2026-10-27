import { execFile } from 'node:child_process';
import { promisify } from 'node:util';

const run = promisify(execFile);

/** Another git process holds a lock for a moment (e.g. two worktrees created at once). */
const TRANSIENT_LOCK = /\.lock.*file exists|could not lock (ref|config)|unable to lock/i;
const LOCK_RETRY_MS = [50, 150, 400];

/**
 * Runs system git in `cwd` and returns trimmed stdout. Output is in English and stable for parsing.
 * A transient lock error is retried a few times (idea of emdash `git-schedule.ts`, Apache-2.0).
 */
export async function git(
  cwd: string,
  args: string[],
  env: NodeJS.ProcessEnv = {},
  opts: { timeoutMs?: number } = {},
): Promise<string> {
  for (let attempt = 0; ; attempt++) {
    try {
      const { stdout } = await run('git', args, {
        cwd,
        maxBuffer: 64 * 1024 * 1024,
        timeout: opts.timeoutMs ?? 0,
        env: { ...process.env, ...env, LC_ALL: 'C', GIT_TERMINAL_PROMPT: '0' },
      });
      return stdout.trimEnd();
    } catch (error) {
      const stderr = String((error as { stderr?: unknown }).stderr ?? '');
      const delay = LOCK_RETRY_MS[attempt];
      if (delay === undefined || !TRANSIENT_LOCK.test(stderr)) throw error;
      await new Promise((resolve) => setTimeout(resolve, delay));
    }
  }
}
