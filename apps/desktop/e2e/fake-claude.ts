import { chmodSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

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

/**
 * A stand-in `claude` on PATH. Without `-p` it plays the interactive TUI of a workspace terminal. With `--json-schema` it answers with an assessment or a plan depending
 * on the schema; without it (an implement run) it adds a section to README.md in its working directory
 * and reports the edit, the way Claude Code would.
 */
export function fakeClaude(): string {
  const bin = mkdtempSync(join(tmpdir(), 'studio-fake-claude-'));
  const data = join(bin, 'answers.json');
  writeFileSync(data, JSON.stringify(answers));
  const script = `#!/usr/bin/env node
const fs = require('node:fs');
const args = process.argv.slice(2);
if (args.includes('--version')) { console.log('2.0.0 (Claude Code)'); process.exit(0); }
const line = (m) => process.stdout.write(JSON.stringify(m) + '\\n');
if (!args.includes('-p')) {
  // Interactive, as in a workspace terminal tab: say who we are, then echo what is typed.
  const id = args.includes('--session-id') ? args[args.indexOf('--session-id') + 1] : 'none';
  process.stdout.write('fake claude, session ' + id + ', in ' + require('node:path').basename(process.cwd()) + '\\r\\n> ');
  require('node:readline').createInterface({ input: process.stdin }).on('line', (text) => {
    if (text === 'exit') process.exit(0);
    process.stdout.write('you said: ' + text + '\\r\\n> ');
  });
  return;
}
const done = (extra) => setTimeout(() => line({ type: 'result', subtype: 'success', session_id: 's', total_cost_usd: 0.12, ...extra }), 300);
if (!args.includes('--json-schema')) {
  fs.appendFileSync('README.md', '\\n## Запуск\\n\\npnpm dev\\n');
  line({ type: 'assistant', message: { content: [{ type: 'tool_use', name: 'Edit', input: { file_path: process.cwd() + '/README.md' } }] } });
  line({ type: 'assistant', message: { content: [{ type: 'text', text: 'Добавил раздел «Запуск» в README.md.' }] } });
  done({ result: 'Добавил раздел «Запуск» в README.md.' });
} else {
  const answers = JSON.parse(fs.readFileSync(${JSON.stringify(data)}, 'utf8'));
  const schema = args[args.indexOf('--json-schema') + 1] || '';
  line({ type: 'assistant', message: { content: [{ type: 'tool_use', name: 'Read', input: { file_path: 'README.md' } }] } });
  done({ structured_output: schema.includes('variants') ? answers.plan : answers.assessment });
}
`;
  const file = join(bin, 'claude');
  writeFileSync(file, script);
  chmodSync(file, 0o755);
  return bin;
}
