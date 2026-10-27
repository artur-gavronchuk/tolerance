import { _electron as electron, expect, test } from '@playwright/test';

test('the app opens a window with the welcome screen', async () => {
  const app = await electron.launch({ args: ['.'], cwd: new URL('..', import.meta.url).pathname });
  const win = await app.firstWindow();
  await expect(win.getByRole('heading', { level: 1 })).toBeVisible();
  await expect(win.getByText('Studio', { exact: true })).toBeVisible();
  if (process.env.STUDIO_SCREENSHOT) await win.screenshot({ path: process.env.STUDIO_SCREENSHOT });
  await app.close();
});
