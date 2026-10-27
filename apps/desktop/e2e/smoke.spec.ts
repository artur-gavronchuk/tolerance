import { expect, test } from '@playwright/test';
import { launchStudio } from './helpers';

test('the app opens a window with the welcome screen', async () => {
  const { app, win } = await launchStudio();
  await expect(win.getByRole('heading', { level: 1 })).toBeVisible();
  await expect(win.getByText('Studio', { exact: true })).toBeVisible();
  if (process.env.STUDIO_SCREENSHOT) await win.screenshot({ path: process.env.STUDIO_SCREENSHOT });
  await app.close();
});
