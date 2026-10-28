import { execFileSync } from 'node:child_process';
import { chmodSync, existsSync, mkdtempSync, readFileSync, realpathSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import { fakeClaude } from './fake-claude';
import { launchStudio, mutate, query } from './helpers';

const git = (cwd: string, ...args: string[]) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim();

/** A stand-in `gh`: no pull request until `pr create`, which records its arguments. */
function fakeGh(bin: string): void {
  writeFileSync(
    join(bin, 'gh'),
    `#!/bin/sh
dir="$(dirname "$0")"
case "$1 $2" in
  "pr list") if [ -f "$dir/pr-created" ]; then echo '[{"url":"https://github.com/o/arena/pull/12"}]'; else echo '[]'; fi ;;
  "pr create") echo "$@" > "$dir/pr-created"; echo "https://github.com/o/arena/pull/12" ;;
  *) exit 1 ;;
esac
`,
  );
  chmodSync(join(bin, 'gh'), 0o755);
}

test('changes → commit → push → pull request, and the agent session continues after a restart', async () => {
  const tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-e2e-changes-')));
  const repo = join(tmp, 'arena');
  const origin = join(tmp, 'origin.git');
  git(tmp, 'init', '-q', '-b', 'main', repo);
  writeFileSync(join(repo, 'README.md'), '# demo\n');
  git(repo, 'add', '-A');
  git(repo, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'init');
  git(tmp, 'init', '-q', '--bare', origin);
  git(repo, 'remote', 'add', 'origin', origin);
  git(repo, 'push', '-q', 'origin', 'main');
  git(repo, 'config', 'user.name', 'Dev');
  git(repo, 'config', 'user.email', 'dev@example.com');

  const bin = fakeClaude();
  fakeGh(bin);
  const env = {
    PATH: `${bin}:${process.env.PATH}`,
    SHELL: '/nonexistent',
    STUDIO_HOME: join(tmp, 'home'),
    STUDIO_WORKTREES_ROOT: join(tmp, 'worktrees'),
  };
  const first = await launchStudio(env);
  let win = first.win;

  await mutate(win, 'projects.add', { path: repo });
  await win.reload();
  await win.locator('aside').getByRole('button', { name: 'arena', exact: true }).click();
  await win.getByRole('button', { name: 'Новый воркспейс' }).click();
  await win.getByLabel('Имя нового воркспейса').fill('login');
  await win.getByLabel('Имя нового воркспейса').press('Enter');
  await expect(win.getByTestId('terminal-agent')).toContainText('fake claude, session');
  const ws = join(tmp, 'worktrees', 'arena', 'ws', 'login');

  // What the agent did shows up on the Changes tab, untracked files included.
  writeFileSync(join(ws, 'README.md'), '# demo\n\nLogin works.\n');
  writeFileSync(join(ws, 'login.ts'), 'export const login = true;\n');
  await win.getByRole('tab', { name: 'Изменения' }).click();
  const state = win.getByTestId('branch-state');
  await expect(state).toContainText('2 файла не закоммичены');
  await expect(state).toContainText('0 коммитов от origin/main');
  await expect(win.getByTestId('changes-diff')).toContainText('+export const login = true;');
  await win.getByRole('button', { name: /login\.ts/ }).click();
  await expect(win.getByTestId('changes-diff')).not.toContainText('Login works.');

  await win.getByLabel('Сообщение коммита').fill('Add login');
  await win.getByRole('button', { name: 'Коммит' }).click();
  await expect(state).toContainText('0 файлов не закоммичены');
  await expect(state).toContainText('1 коммит от origin/main');
  await expect(state).toContainText('ветка ещё не отправлена');
  expect(git(ws, 'log', '-1', '--format=%s %an')).toBe('Add login Dev');

  await win.getByRole('button', { name: 'Push' }).click();
  await expect(state).toContainText('всё отправлено');
  expect(git(origin, 'rev-parse', 'studio/ws/login')).toBe(git(ws, 'rev-parse', 'HEAD'));

  await win.getByRole('button', { name: 'Создать PR в main' }).click();
  await expect(win.getByRole('link', { name: 'Открыть PR #12' })).toBeVisible();
  expect(readFileSync(join(bin, 'pr-created'), 'utf8').trim()).toBe(
    'pr create --head studio/ws/login --base main --fill',
  );
  // Nothing was merged anywhere: main is where it was, locally and on the server.
  expect(git(origin, 'rev-parse', 'main')).toBe(git(repo, 'rev-parse', 'main'));
  expect(git(repo, 'log', '-1', '--format=%s', 'main')).toBe('init');

  // Restart: the workspace is still there, and its agent continues the same Claude session.
  const [project] = (await query(win, 'projects.list', undefined)) as { id: string }[];
  const [item] = (await query(win, 'workspaces.list', { projectId: project?.id })) as {
    workspace: { id: string };
  }[];
  const tabs = (await query(win, 'terminals.list', { workspaceId: item?.workspace.id })) as {
    terminal: { kind: string; sessionId: string };
  }[];
  const session = tabs.find((x) => x.terminal.kind === 'agent')?.terminal.sessionId;
  await first.app.close();

  const second = await launchStudio(env, { userData: first.userData });
  win = second.win;
  await win
    .locator('aside')
    .getByRole('button', { name: /^login/ })
    .click();
  await win.getByRole('tab', { name: 'claude' }).click();
  await win.getByRole('button', { name: 'Продолжить сессию' }).click();
  await expect(win.getByTestId('terminal-agent')).toContainText(
    `fake claude, resumed session ${session}, in login`,
  );
  expect(existsSync(ws)).toBe(true);
  await second.app.close();
});
