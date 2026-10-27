import { execFileSync } from 'node:child_process';
import { expect, test } from '@playwright/test';
import { launchStudio, mutate } from './helpers';

// Uses the real Claude Code and costs money, so it only runs when asked:
//   STUDIO_E2E_REPO=/path/to/repo pnpm test:e2e real-agent
const repo = process.env.STUDIO_E2E_REPO;

test('a repository is scanned and mapped by the agent', async () => {
  test.skip(!repo, 'set STUDIO_E2E_REPO to run against a real repository');
  test.setTimeout(15 * 60_000);
  const { app, win } = await launchStudio();
  const shots = process.env.STUDIO_SCREENSHOT_DIR;

  const { project } = (await mutate(win, 'projects.add', { path: repo })) as { project: { id: string } };
  await mutate(win, 'map.start', { projectId: project.id, language: 'ru' });
  await win.reload();
  await win
    .getByRole('button', { name: /arena|tolerance/ })
    .first()
    .click();

  await expect(win.getByText('Агент изучает проект')).toBeVisible();
  await win.waitForTimeout(20_000);
  if (shots) await win.screenshot({ path: `${shots}/map-running.png` });

  await expect(win.getByText(/Построена/)).toBeVisible({ timeout: 14 * 60_000 });
  if (shots) await win.screenshot({ path: `${shots}/map-done.png`, fullPage: true });
  await app.close();
});

test('a task written in the UI is assessed by the agent', async () => {
  test.skip(!repo, 'set STUDIO_E2E_REPO to run against a real repository');
  test.setTimeout(10 * 60_000);
  const { app, win } = await launchStudio();
  const shots = process.env.STUDIO_SCREENSHOT_DIR;

  await mutate(win, 'projects.add', { path: repo });
  await win.reload();
  await win
    .locator('aside')
    .getByRole('button', { name: repo?.split('/').filter(Boolean).pop() })
    .click();

  const box = win.getByRole('textbox', { name: 'Новая задача' });
  await expect(box).toBeFocused();
  await box.fill(process.env.STUDIO_E2E_TASK ?? 'Добавить README-раздел о том, как запустить проект');
  if (shots) await win.screenshot({ path: `${shots}/task-new.png` });
  await win.getByRole('button', { name: 'Отдать агенту' }).click();

  await expect(win.getByText('Агент изучает задачу')).toBeVisible();
  await win.waitForTimeout(15_000);
  if (shots) await win.screenshot({ path: `${shots}/task-running.png` });

  await expect(win.getByText('Мнение агента')).toBeVisible({ timeout: 9 * 60_000 });
  if (shots) await win.screenshot({ path: `${shots}/task-assessed.png`, fullPage: true });
  await app.close();
});

test('a task of the approved plan is implemented by the agent in a worktree', async () => {
  test.skip(!repo, 'set STUDIO_E2E_REPO to run against a real repository');
  test.setTimeout(30 * 60_000);
  const git = (...args: string[]) => execFileSync('git', args, { cwd: repo, encoding: 'utf8' }).trim();
  const mainHead = git('rev-parse', 'HEAD');
  const { app, win } = await launchStudio();
  const shots = process.env.STUDIO_SCREENSHOT_DIR;

  const { project } = (await mutate(win, 'projects.add', { path: repo })) as { project: { id: string } };
  await mutate(win, 'requests.create', {
    projectId: project.id,
    text: process.env.STUDIO_E2E_TASK ?? 'Добавь в README раздел «Как запустить тесты»',
    language: 'ru',
  });
  await win.reload();
  await win
    .locator('aside')
    .getByRole('button', { name: repo?.split('/').filter(Boolean).pop() })
    .click();

  await expect(win.getByText('Мнение агента')).toBeVisible({ timeout: 9 * 60_000 });
  await win.getByRole('button', { name: 'Составить план' }).click();
  await win
    .getByRole('button', { name: 'Утвердить этот вариант' })
    .first()
    .click({ timeout: 9 * 60_000 });

  const first = win.locator('[data-task]').first();
  await first.getByRole('button', { name: 'Выполнить', exact: true }).click();
  await win.waitForTimeout(30_000);
  if (shots) await win.screenshot({ path: `${shots}/implement-running.png`, fullPage: true });

  await expect(first.locator('pre')).toBeVisible({ timeout: 15 * 60_000 });
  if (shots) await win.screenshot({ path: `${shots}/implement-done.png`, fullPage: true });
  expect(git('rev-parse', 'HEAD')).toBe(mainHead);
  expect(git('status', '--porcelain')).toBe('');
  await app.close();
});
