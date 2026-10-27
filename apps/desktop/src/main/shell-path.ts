import { execFile } from 'node:child_process';

const MARK = '__STUDIO_PATH__';

/**
 * Apps started from the Dock or Finder get a minimal PATH, so `claude`, `git` from Homebrew and
 * friends are not found. Take PATH from the user's login shell once at startup.
 */
export async function adoptLoginShellPath(): Promise<void> {
  if (process.platform === 'win32') return;
  const shell = process.env.SHELL || '/bin/zsh';
  const path = await new Promise<string | null>((resolve) => {
    execFile(shell, ['-ilc', `printf '${MARK}%s${MARK}' "$PATH"`], { timeout: 5000 }, (error, stdout) => {
      resolve(error ? null : (new RegExp(`${MARK}(.*)${MARK}`).exec(stdout)?.[1] ?? null));
    });
  });
  if (path) process.env.PATH = path;
}
