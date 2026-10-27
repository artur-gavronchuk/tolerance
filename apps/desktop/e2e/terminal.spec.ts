import { execFileSync } from 'node:child_process';
import { mkdtempSync, realpathSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import { fakeClaude } from './fake-claude';
import { launchStudio, mutate, query } from './helpers';

const git = (cwd: string, ...args: string[]) => execFileSync('git', args, { cwd, encoding: 'utf8' }).trim();

test('a workspace opens with the agent and a shell that keep running across switches and reloads', async () => {
  const tmp = realpathSync(mkdtempSync(join(tmpdir(), 'studio-e2e-term-')));
  const repo = join(tmp, 'arena');
  git(tmp, 'init', '-q', '-b', 'main', repo);
  writeFileSync(join(repo, 'README.md'), '# demo\n');
  git(repo, 'add', '-A');
  git(repo, '-c', 'user.name=t', '-c', 'user.email=t@t', 'commit', '-qm', 'init');
  const worktrees = realpathSync(mkdtempSync(join(tmpdir(), 'studio-e2e-worktrees-')));

  const bin = fakeClaude();
  const { app, win } = await launchStudio({
    PATH: `${bin}:${process.env.PATH}`,
    // No login shell anywhere: it would reorder PATH and pick the real claude over the fake one.
    // Terminals then fall back to a plain interactive /bin/sh.
    SHELL: '/nonexistent',
    PS1: '$ ',
    ENV: '',
    STUDIO_WORKTREES_ROOT: worktrees,
  });
  const shots = process.env.STUDIO_SCREENSHOT_DIR;

  await mutate(win, 'projects.add', { path: repo });
  await win.reload();
  await win.locator('aside').getByRole('button', { name: 'arena', exact: true }).click();
  await win.getByRole('button', { name: 'Новый воркспейс' }).click();
  await win.getByLabel('Имя нового воркспейса').fill('login');
  await win.getByLabel('Имя нового воркспейса').press('Enter');

  // The agent tab is open and started with the session id Studio stored.
  const agent = win.getByTestId('terminal-agent');
  await expect(win.getByRole('tab', { name: 'claude' })).toHaveAttribute('aria-selected', 'true');
  await expect(agent).toContainText(/fake claude, session [0-9a-f-]{36}, in login/);
  const [project] = (await query(win, 'projects.list', undefined)) as { id: string }[];
  const [ws] = (await query(win, 'workspaces.list', { projectId: project?.id })) as {
    workspace: { id: string };
  }[];
  const tabs = async () =>
    (await query(win, 'terminals.list', { workspaceId: ws?.workspace.id })) as {
      terminal: { kind: string; sessionId: string | null };
    }[];
  const firstSession = (await tabs()).find((x) => x.terminal.kind === 'agent')?.terminal.sessionId;
  await expect(agent).toContainText(`session ${firstSession}`);

  await agent.click();
  await win.keyboard.type('hello');
  await win.keyboard.press('Enter');
  await expect(agent).toContainText('you said: hello');
  if (shots) await win.screenshot({ path: `${shots}/terminal-agent.png` });

  // The shell tab runs in the worktree with the workspace's variables.
  await win.getByRole('tab', { name: 'shell' }).click();
  const shell = win.getByTestId('terminal-shell');
  await shell.click();
  await win.keyboard.type('echo "on $STUDIO_BRANCH in $(basename "$PWD")"');
  await win.keyboard.press('Enter');
  await expect(shell).toContainText('on studio/ws/login in login');

  // Leaving the workspace and coming back keeps the agent's screen and process.
  await win.locator('aside').getByRole('button', { name: 'arena', exact: true }).click();
  await expect(win.getByTestId('terminal-agent')).toHaveCount(0);
  await win.locator('aside').getByRole('button', { name: /login/ }).click();
  await win.getByRole('tab', { name: 'claude' }).click();
  await expect(agent).toContainText('you said: hello');
  await agent.click();
  await win.keyboard.type('still here');
  await win.keyboard.press('Enter');
  await expect(agent).toContainText('you said: still here');

  // A window reload rebuilds the screen from main's buffer.
  await win.reload();
  await win.getByRole('tab', { name: 'claude' }).click();
  await expect(agent).toContainText('you said: still here');

  // More shells can be opened and closed.
  await win.getByRole('button', { name: 'Новый shell' }).click();
  await expect(win.getByRole('tab', { name: 'shell 2' })).toHaveAttribute('aria-selected', 'true');
  await win.getByRole('button', { name: 'Закрыть shell 2' }).click();
  await expect(win.getByRole('tab', { name: 'shell 2' })).toHaveCount(0);

  // When the agent exits, it can be started again with a new session.
  await win.getByRole('tab', { name: 'claude' }).click();
  await agent.click();
  await win.keyboard.type('exit');
  await win.keyboard.press('Enter');
  await expect(win.getByText('Процесс завершился, код 0.')).toBeVisible();
  await win.getByRole('button', { name: 'Запустить claude' }).click();
  const secondSession = async () =>
    (await tabs()).find((x) => x.terminal.kind === 'agent')?.terminal.sessionId;
  await expect.poll(secondSession).not.toBe(firstSession);
  await expect(agent).toContainText(`session ${await secondSession()}`);
  await app.close();
});
