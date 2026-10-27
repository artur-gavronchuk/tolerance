import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, realpathSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import { launchStudio, mutate } from './helpers';

const git = (cwd: string, ...args: string[]) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim();
const identity = ['-c', 'user.name=t', '-c', 'user.email=t@t'];

test('a workspace is a fresh branch + worktree with setup output, deleted with a warning, branch kept', async () => {
  const tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-e2e-ws-')));
  const repo = join(tmp, 'arena');
  const origin = join(tmp, 'origin.git');
  const other = join(tmp, 'other');
  git(tmp, 'init', '-q', '-b', 'main', repo);
  writeFileSync(join(repo, 'README.md'), '# demo\n');
  writeFileSync(join(repo, '.gitignore'), '.env\n');
  writeFileSync(join(repo, '.env'), 'TOKEN=local\n');
  git(repo, 'add', '-A');
  git(repo, ...identity, 'commit', '-qm', 'init');
  git(tmp, 'clone', '-q', '--bare', repo, origin);
  git(repo, 'remote', 'add', 'origin', origin);
  git(repo, 'fetch', '-q', 'origin');
  // A teammate pushed to main; the local main is behind.
  git(tmp, 'clone', '-q', origin, other);
  writeFileSync(join(other, 'news.md'), 'fresh\n');
  git(other, 'add', '-A');
  git(other, ...identity, 'commit', '-qm', 'news');
  git(other, 'push', '-q', 'origin', 'main');
  const localMain = git(repo, 'rev-parse', 'main');
  const freshMain = git(other, 'rev-parse', 'HEAD');

  const worktrees = realpathSync(mkdtempSync(join(tmpdir(), 'studio-e2e-worktrees-')));
  const { app, win } = await launchStudio({ SHELL: '/nonexistent', STUDIO_WORKTREES_ROOT: worktrees });
  const shots = process.env.STUDIO_SCREENSHOT_DIR;

  await mutate(win, 'projects.add', { path: repo });
  await win.reload();
  await win.locator('aside').getByRole('button', { name: 'arena', exact: true }).click();

  // Project settings: the setup command for new workspaces and an editor that does not exist.
  await win
    .getByLabel('Команда setup')
    .fill('cp "$STUDIO_ROOT_PATH/.env" .env && echo "setup ok on $STUDIO_BRANCH"');
  await win.getByLabel('Редактор').fill('no-such-editor-xyz');
  await win.getByRole('button', { name: 'Сохранить' }).click();
  await expect(win.getByText('Сохранено')).toBeVisible();

  await win.getByRole('button', { name: 'Новый воркспейс' }).click();
  await win.getByLabel('Имя нового воркспейса').fill('Починить логин');
  await win.getByLabel('Имя нового воркспейса').press('Enter');

  await expect(win.getByRole('heading', { name: 'Починить логин' })).toBeVisible();
  await expect(win.getByText('studio/ws/pochinit-login').first()).toBeVisible();
  await expect(win.getByText(`от origin/main · ${freshMain.slice(0, 7)}`)).toBeVisible();
  await expect(win.getByTestId('setup-log')).toContainText('setup ok on studio/ws/pochinit-login');
  await expect(win.getByText('готово')).toBeVisible();
  if (shots) await win.screenshot({ path: `${shots}/workspace.png` });

  const path = join(worktrees, 'arena', 'ws', 'pochinit-login');
  expect(git(path, 'rev-parse', 'HEAD')).toBe(freshMain);
  expect(readFileSync(join(path, '.env'), 'utf8')).toBe('TOKEN=local\n');
  // The user's checkout and local main are untouched.
  expect(git(repo, 'rev-parse', 'main')).toBe(localMain);
  expect(git(repo, 'rev-parse', '--abbrev-ref', 'HEAD')).toBe('main');
  expect(git(repo, 'status', '--porcelain')).toBe('');

  await win.getByRole('button', { name: 'Редактор', exact: true }).click();
  await expect(win.getByText('Не получилось запустить редактор «no-such-editor-xyz»')).toBeVisible();

  // Deleting warns about uncommitted work and keeps the branch.
  writeFileSync(join(path, 'draft.txt'), 'wip\n');
  await win.getByRole('button', { name: 'Удалить', exact: true }).click();
  const dialog = win.getByRole('alertdialog');
  await expect(dialog.getByText('1 файл с незакоммиченными изменениями пропадёт:')).toBeVisible();
  await expect(dialog.getByText('draft.txt')).toBeVisible();
  if (shots) await win.screenshot({ path: `${shots}/workspace-delete.png` });
  await dialog.getByRole('button', { name: 'Удалить воркспейс' }).click();

  await expect(win.getByRole('heading', { name: 'Починить логин' })).toHaveCount(0);
  await expect(win.locator('aside').getByText('Пока ни одного')).toBeVisible();
  expect(existsSync(path)).toBe(false);
  expect(git(repo, 'branch', '--list', 'studio/ws/*', '--format=%(refname:short)')).toBe(
    'studio/ws/pochinit-login',
  );
  await app.close();
});
