import { execFile, spawn } from 'node:child_process';
import { shell } from 'electron';

/** Why a folder could not be opened; `code` is an i18n key for the UI, `detail` names the command. */
export class OpenInError extends Error {
  constructor(
    readonly code: 'editor_not_found' | 'terminal_not_found' | 'folder_failed',
    readonly detail: string,
  ) {
    super(`${code}: ${detail}`);
  }
}

export const DEFAULT_EDITOR = 'code';
export const DEFAULT_TERMINAL = process.platform === 'darwin' ? 'Terminal' : 'x-terminal-emulator';

/**
 * macOS apps behind the editor CLIs, for when the CLI is not on PATH (VS Code's "Install 'code'
 * command" was never run, IntelliJ without Toolbox scripts). The `||` fallback idea is from emdash
 * `open-in-apps.ts` (Apache-2.0).
 */
const EDITOR_APPS: Record<string, string[]> = {
  code: ['Visual Studio Code'],
  cursor: ['Cursor'],
  idea: ['IntelliJ IDEA', 'IntelliJ IDEA Ultimate', 'IntelliJ IDEA CE'],
};

/** Starts a GUI program and returns once it has started; fails when the executable is missing. */
function launch(command: string, args: string[], cwd?: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { cwd, detached: true, stdio: 'ignore' });
    child.once('error', reject);
    child.once('spawn', () => {
      child.unref();
      resolve();
    });
  });
}

/** `open -a <app> <path>`; fails when there is no such app. */
function openApp(app: string, path: string): Promise<void> {
  return new Promise((resolve, reject) => {
    execFile('/usr/bin/open', ['-a', app, path], (error) => (error ? reject(error) : resolve()));
  });
}

/**
 * Opens `path` in the editor. `command` is a CLI with optional arguments split on spaces, e.g.
 * `cursor` or `code --new-window`; the path goes last.
 */
export async function openInEditor(command: string | null, path: string): Promise<void> {
  const [cli = DEFAULT_EDITOR, ...args] = (command?.trim() || DEFAULT_EDITOR).split(/\s+/);
  try {
    await launch(cli, [...args, path]);
    return;
  } catch {
    // Falls through to the app bundle on macOS.
  }
  if (process.platform === 'darwin') {
    for (const app of EDITOR_APPS[cli] ?? []) {
      try {
        await openApp(app, path);
        return;
      } catch {
        // Try the next name.
      }
    }
  }
  throw new OpenInError('editor_not_found', cli);
}

/** Opens a terminal window in `path`: on macOS an app by name, elsewhere a command started in the folder. */
export async function openInTerminal(app: string | null, path: string): Promise<void> {
  const name = app?.trim() || DEFAULT_TERMINAL;
  try {
    if (process.platform === 'darwin') await openApp(name, path);
    else await launch(name, [], path);
  } catch {
    throw new OpenInError('terminal_not_found', name);
  }
}

export async function openFolder(path: string): Promise<void> {
  const error = await shell.openPath(path);
  if (error) throw new OpenInError('folder_failed', error);
}
