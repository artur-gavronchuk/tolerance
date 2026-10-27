import { execFileSync } from 'node:child_process';
import { existsSync, mkdtempSync, readFileSync, realpathSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import { fakeClaude } from './fake-claude';
import { launchStudio, mutate } from './helpers';

const git = (cwd: string, ...args: string[]) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim();

test('a task of the approved plan runs in its own worktree and leaves the main branch alone', async () => {
  const repo = realpathSync(mkdtempSync(join(tmpdir(), 'studio-e2e-impl-')));
  writeFileSync(join(repo, 'README.md'), '# demo\n');
  git(repo, 'init', '-q', '-b', 'main');
  git(repo, 'add', '-A');
  git(repo, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'init');
  const mainHead = git(repo, 'rev-parse', 'HEAD');
  const worktrees = realpathSync(mkdtempSync(join(tmpdir(), 'studio-e2e-worktrees-')));

  const bin = fakeClaude();
  const { app, win } = await launchStudio({
    PATH: `${bin}:${process.env.PATH}`,
    SHELL: '/nonexistent',
    STUDIO_WORKTREES_ROOT: worktrees,
  });
  const shots = process.env.STUDIO_SCREENSHOT_DIR;

  const { project } = (await mutate(win, 'projects.add', { path: repo })) as { project: { id: string } };
  await mutate(win, 'requests.create', {
    projectId: project.id,
    text: 'Добавь раздел в README',
    language: 'ru',
  });
  await win.reload();
  await win.locator('aside').getByRole('button').filter({ hasText: 'studio-e2e-impl' }).click();

  await expect(win.getByText('Мнение агента')).toBeVisible();
  await win.getByRole('button', { name: 'Составить план' }).click();
  await win.getByRole('button', { name: 'Утвердить этот вариант' }).first().click();

  const t1 = win.locator('[data-task="T1"]');
  await expect(t1.getByText('не запускалась')).toBeVisible();
  await t1.getByRole('button', { name: 'Выполнить', exact: true }).click();

  await expect(t1.getByText('готово')).toBeVisible();
  await expect(t1.getByText('Добавил раздел «Запуск» в README.md.')).toBeVisible();
  await expect(t1.locator('pre').getByText('+## Запуск')).toBeVisible();
  await expect(t1.getByText('README.md', { exact: true })).toBeVisible();
  await expect(win.getByRole('button', { name: 'Вернуть к выбору' })).toHaveCount(0);
  if (shots) await win.screenshot({ path: `${shots}/task-done.png`, fullPage: true });

  // The work is on a studio/* branch in a worktree under the Studio folder; the user's checkout is untouched.
  const branch = git(repo, 'branch', '--list', 'studio/*', '--format=%(refname:short)');
  expect(branch).toMatch(/^studio\/[0-9a-f]{8}\/T1-shag-t1$/);
  expect(git(repo, 'log', '-1', '--format=%s', branch)).toBe('studio: T1 Шаг T1');
  expect(git(repo, 'rev-parse', 'HEAD')).toBe(mainHead);
  expect(git(repo, 'rev-parse', '--abbrev-ref', 'HEAD')).toBe('main');
  expect(git(repo, 'status', '--porcelain')).toBe('');
  expect(readFileSync(join(repo, 'README.md'), 'utf8')).toBe('# demo\n');
  const worktree = git(repo, 'worktree', 'list', '--porcelain')
    .split('\n')
    .find((l) => l.startsWith('worktree ') && l.includes(worktrees))
    ?.slice('worktree '.length);
  expect(worktree && existsSync(worktree)).toBe(true);

  await t1.getByRole('button', { name: 'Удалить worktree' }).click();
  await expect(t1.getByText('удалён, изменения показаны по ветке')).toBeVisible();
  await expect(t1.locator('pre').getByText('+## Запуск')).toBeVisible();
  expect(existsSync(worktree ?? '')).toBe(false);
  expect(git(repo, 'branch', '--list', 'studio/*', '--format=%(refname:short)')).toBe(branch);
  await app.close();
});
