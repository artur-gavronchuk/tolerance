import { execFileSync } from 'node:child_process';
import { chmodSync, mkdirSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from '@playwright/test';
import { launchStudio, mutate } from './helpers';

const task = (key: string, dependsOn: string[]) => ({
  key,
  title: `Шаг ${key}`,
  description: 'Описание шага',
  dependsOn,
  scope: { allow: [`src/${key}/**`], deny: [] },
  acceptance: [{ id: `${key}.A1`, text: 'Тест проходит', check: 'command', command: 'npm test' }],
  agentHint: 'claude',
});
const variant = (title: string, tasks: unknown[]) => ({
  title,
  approach: 'Суть подхода в нескольких предложениях.',
  tradeoffs: { pros: ['Быстро'], cons: ['Временное решение'], risks: [] },
  complexity: 'S',
  affectedComponents: [],
  tasks,
});
const answers = {
  assessment: {
    understanding: 'Нужен раздел в README',
    verdict: 'go',
    opinion: 'Простая задача.',
    complexity: 'S',
    complexityReason: 'Один файл',
    affected: [{ path: 'README.md', note: 'новый раздел' }],
    approach: ['Написать раздел'],
    risks: [],
    questions: [],
  },
  plan: {
    variants: [
      variant('Дописать README', [task('T1', [])]),
      variant('Отдельный документ', [task('T1', []), task('T2', ['T1'])]),
    ],
  },
};

/** A stand-in `claude` on PATH: answers with an assessment or a plan depending on the schema it is given. */
function fakeClaude(): string {
  const bin = mkdtempSync(join(tmpdir(), 'studio-fake-claude-'));
  const data = join(bin, 'answers.json');
  writeFileSync(data, JSON.stringify(answers));
  const script = `#!/usr/bin/env node
const args = process.argv.slice(2);
if (args.includes('--version')) { console.log('2.0.0 (Claude Code)'); process.exit(0); }
const answers = JSON.parse(require('node:fs').readFileSync(${JSON.stringify(data)}, 'utf8'));
const schema = args[args.indexOf('--json-schema') + 1] || '';
const output = schema.includes('variants') ? answers.plan : answers.assessment;
const line = (m) => process.stdout.write(JSON.stringify(m) + '\\n');
line({ type: 'assistant', message: { content: [{ type: 'tool_use', name: 'Read', input: { file_path: 'README.md' } }] } });
setTimeout(() => line({ type: 'result', subtype: 'success', structured_output: output, session_id: 's', total_cost_usd: 0.12 }), 300);
`;
  const file = join(bin, 'claude');
  writeFileSync(file, script);
  chmodSync(file, 0o755);
  return bin;
}

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
