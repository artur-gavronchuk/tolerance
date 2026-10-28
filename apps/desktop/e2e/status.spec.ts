import { execFileSync } from 'node:child_process';
import { mkdtempSync, readdirSync, realpathSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import { fakeClaude } from './fake-claude';
import { launchStudio, mutate } from './helpers';

const git = (cwd: string, ...args: string[]) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim();

test('the sidebar shows which agent waits, and opening the workspace answers it', async () => {
  const tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-e2e-status-')));
  const repo = join(tmp, 'arena');
  git(tmp, 'init', '-q', '-b', 'main', repo);
  writeFileSync(join(repo, 'README.md'), '# demo\n');
  git(repo, 'add', '-A');
  git(repo, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'init');
  const home = join(tmp, 'studio-home');

  const { app, win } = await launchStudio({
    PATH: `${fakeClaude()}:${process.env.PATH}`,
    SHELL: '/nonexistent',
    STUDIO_HOME: home,
    STUDIO_WORKTREES_ROOT: join(tmp, 'worktrees'),
  });
  // The window counts as focused even when the test runs in the background.
  const cdp = await win.context().newCDPSession(win);
  await cdp.send('Emulation.setFocusEmulationEnabled', { enabled: true });

  // The hook settings live in STUDIO_HOME, one file per running Studio.
  expect(readdirSync(home).filter((f) => /^claude-hooks-\d+\.json$/.test(f))).toHaveLength(1);

  await mutate(win, 'projects.add', { path: repo });
  await win.reload();
  const sidebar = win.locator('aside');
  await sidebar.getByRole('button', { name: 'arena', exact: true }).click();
  for (const name of ['one', 'two']) {
    await win.getByRole('button', { name: 'Новый воркспейс' }).click();
    await win.getByLabel('Имя нового воркспейса').fill(name);
    await win.getByLabel('Имя нового воркспейса').press('Enter');
    await expect(win.getByTestId('terminal-agent')).toContainText(`in ${name}`);
  }
  const item = (name: string) => sidebar.getByRole('button', { name: new RegExp(`^${name}`) });
  const dot = (name: string) => item(name).getByRole('img');
  const counter = sidebar.getByRole('button', { name: /^(ждёт|ждут) \d+$/ });

  // In "two" the agent will ask for a permission in a moment; meanwhile the developer goes to "one".
  const agent = win.getByTestId('terminal-agent');
  await agent.click();
  await win.keyboard.type('ask later');
  await win.keyboard.press('Enter');
  await expect(dot('two')).toHaveAttribute('data-status', 'working');
  await item('one').click();
  await expect(win.getByRole('heading', { name: 'one' })).toBeVisible();

  await expect(dot('two')).toHaveAttribute('data-status', 'needs_input');
  await expect(counter).toHaveText('ждёт 1');
  if (process.env.STUDIO_SCREENSHOT_DIR) {
    await win.screenshot({ path: `${process.env.STUDIO_SCREENSHOT_DIR}/status-waiting.png` });
  }

  // The counter leads there; answering makes it work, then finish, with nothing left to see.
  await counter.click();
  await expect(win.getByRole('heading', { name: 'two' })).toBeVisible();
  await expect(agent).toContainText('May I run rm -rf build?');
  await agent.click();
  await win.keyboard.type('yes');
  await win.keyboard.press('Enter');
  await expect(agent).toContainText('ran it');
  await expect(dot('two')).toHaveAttribute('data-status', 'idle');
  await expect(dot('two')).toHaveAttribute('data-attention', 'false');
  await expect(counter).toHaveCount(0);

  // A turn that ends while the developer watches does not call them back either.
  await item('one').click();
  await agent.click();
  await win.keyboard.type('hello');
  await win.keyboard.press('Enter');
  await expect(agent).toContainText('you said: hello');
  await expect(dot('one')).toHaveAttribute('data-status', 'idle');
  await expect(counter).toHaveCount(0);

  await app.close();
  // The settings file goes with the app.
  expect(readdirSync(home).filter((f) => f.startsWith('claude-hooks-'))).toEqual([]);
});
