import { execFileSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import { fakeClaude } from './fake-claude';
import { launchStudio, mutate } from './helpers';

test('a task gets two plan variants and one is approved', async () => {
  const repo = mkdtempSync(join(tmpdir(), 'studio-e2e-repo-'));
  mkdirSync(join(repo, 'src'));
  writeFileSync(join(repo, 'README.md'), '# demo\n');
  execFileSync('git', ['init', '-q', '-b', 'main'], { cwd: repo });

  const bin = fakeClaude();
  // A missing SHELL keeps this PATH instead of the login shell's, so the fake claude is found first.
  const { app, win } = await launchStudio({ PATH: `${bin}:${process.env.PATH}`, SHELL: '/nonexistent' });
  const shots = process.env.STUDIO_SCREENSHOT_DIR;

  const { project } = (await mutate(win, 'projects.add', { path: repo })) as { project: { id: string } };
  await mutate(win, 'requests.create', {
    projectId: project.id,
    text: 'Добавить раздел про запуск',
    language: 'ru',
  });
  await win.reload();
  await win.locator('aside').getByRole('button').filter({ hasText: 'studio-e2e-repo' }).click();

  await expect(win.getByText('Мнение агента')).toBeVisible();
  await win.getByRole('button', { name: 'Составить план' }).click();

  await expect(win.getByText('Дописать README')).toBeVisible();
  await expect(win.getByText('Отдельный документ')).toBeVisible();
  await expect(win.getByText('после T1')).toBeVisible();
  await win.getByText('Шаг T2').click();
  await expect(win.getByText('src/T2/**')).toBeVisible();
  if (shots) await win.screenshot({ path: `${shots}/plan-variants.png`, fullPage: true });

  await win.getByRole('button', { name: 'Утвердить этот вариант' }).nth(1).click();
  await expect(win.getByText('✓ Утверждён')).toBeVisible();
  await expect(win.getByRole('button', { name: 'Утвердить этот вариант' })).toHaveCount(0);
  if (shots) await win.screenshot({ path: `${shots}/plan-approved.png`, fullPage: true });

  await win.getByRole('button', { name: 'Вернуть к выбору' }).click();
  await expect(win.getByRole('button', { name: 'Утвердить этот вариант' })).toHaveCount(2);
  await app.close();
});
