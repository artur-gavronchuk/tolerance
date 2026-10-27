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
