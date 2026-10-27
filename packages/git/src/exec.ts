import { execFile } from 'node:child_process';
import { promisify } from 'node:util';

const run = promisify(execFile);

/** Runs system git in `cwd` and returns trimmed stdout. Output is in English and stable for parsing. */
export async function git(cwd: string, args: string[], env: NodeJS.ProcessEnv = {}): Promise<string> {
  const { stdout } = await run('git', args, {
    cwd,
    maxBuffer: 64 * 1024 * 1024,
    env: { ...process.env, ...env, LC_ALL: 'C', GIT_TERMINAL_PROMPT: '0' },
  });
  return stdout.trimEnd();
}
