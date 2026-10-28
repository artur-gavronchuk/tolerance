import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, realpathSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import { fakeClaude } from './fake-claude';
import { launchStudio, mutate } from './helpers';

const git = (cwd: string, ...args: string[]) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim();

test('a pain is written from the terminal with the hotkey, and typing goes on where it was', async () => {
  const tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-e2e-pain-')));
  const repo = join(tmp, 'arena');
  git(tmp, 'init', '-q', '-b', 'main', repo);
  writeFileSync(join(repo, 'README.md'), '# demo\n');
  git(repo, 'add', '-A');
  git(repo, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'init');
  const home = join(tmp, 'home');
  const pains = join(home, 'pains.md');

  const { app, win } = await launchStudio({
    PATH: `${fakeClaude()}:${process.env.PATH}`,
    SHELL: '/nonexistent',
    STUDIO_HOME: home,
    STUDIO_WORKTREES_ROOT: join(tmp, 'worktrees'),
  });

  // Outside a project: the button works, the line says so.
  await win.getByRole('button', { name: /^Боль/ }).click();
  const box = win.getByRole('dialog', { name: 'Что мешает?' });
  await expect(box.getByTestId('pain-where')).toHaveText('вне проекта');
  await box.getByRole('textbox').fill('нет списка последних проектов');
  await box.getByRole('textbox').press('Enter');
  await expect(box).toHaveCount(0);
  await expect(win.getByText('Боль записана')).toBeVisible();
  expect(readFileSync(pains, 'utf8')).toMatch(
    /^- \d{4}-\d\d-\d\d \d\d:\d\d · — · — · нет списка последних проектов\n$/,
  );
  // Opening the file is a no-op in tests: nothing pops up on the machine.
  await win.getByRole('button', { name: 'Открыть pains.md' }).click();

  await mutate(win, 'projects.add', { path: repo });
  await win.reload();
  await win.locator('aside').getByRole('button', { name: 'arena', exact: true }).click();
  await win.getByRole('button', { name: 'Новый воркспейс' }).click();
  await win.getByLabel('Имя нового воркспейса').fill('login');
  await win.getByLabel('Имя нового воркспейса').press('Enter');
  const agent = win.getByTestId('terminal-agent');
  await expect(agent).toContainText('fake claude, session');

  // Typing to the agent; the hotkey does not reach the terminal, and Esc keeps the draft.
  await agent.click();
  await win.keyboard.type('fix');
  await win.keyboard.press('ControlOrMeta+Shift+KeyB');
  await expect(box.getByTestId('pain-where')).toHaveText('arena · login');
  await win.keyboard.type('diff не обновился');
  await win.keyboard.press('Escape');
  await expect(box).toHaveCount(0);
  await win.keyboard.press('ControlOrMeta+Shift+KeyB');
  await expect(box.getByRole('textbox')).toHaveValue('diff не обновился');
  await win.keyboard.type(' после коммита');
  await win.keyboard.press('Enter');
  await expect(box).toHaveCount(0);

  const lines = readFileSync(pains, 'utf8').trimEnd().split('\n');
  expect(lines).toHaveLength(2);
  expect(lines[1]).toMatch(/^- \d{4}-\d\d-\d\d \d\d:\d\d · arena · login · diff не обновился после коммита$/);

  // Focus is back in the terminal: the rest of the line goes to the agent.
  await win.keyboard.type('ed');
  await win.keyboard.press('Enter');
  await expect(agent).toContainText('you said: fixed');
  await app.close();
});
