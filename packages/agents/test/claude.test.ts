import { mkdtempSync, readFileSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import type { NormalizedEvent } from '@studio/schema';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { AgentFailedError, runClaudeStructured } from '../src';

const fake = fileURLToPath(new URL('./fake-claude.mjs', import.meta.url));

describe('runClaudeStructured', () => {
  let dir: string;
  beforeEach(() => {
    dir = realpathSync(mkdtempSync(join(tmpdir(), 'studio-agent-')));
  });
  afterEach(() => rmSync(dir, { recursive: true, force: true }));

  function env(script: unknown[]) {
    const file = join(dir, 'script.json');
    writeFileSync(file, JSON.stringify(script));
    return { ...process.env, FAKE_CLAUDE_SCRIPT: file, FAKE_CLAUDE_ARGS: join(dir, 'args.jsonl') };
  }

  it('normalises progress and returns the structured output', async () => {
    const events: NormalizedEvent[] = [];
    const result = await runClaudeStructured({
      cwd: dir,
      prompt: 'map it',
      jsonSchema: { type: 'object' },
      command: fake,
      env: env([
        { type: 'system', subtype: 'init' },
        {
          type: 'assistant',
          message: {
            content: [
              { type: 'text', text: 'Looking around' },
              { type: 'tool_use', name: 'Read', input: { file_path: '$CWD/backend/go.mod' } },
              { type: 'tool_use', name: 'Grep', input: { pattern: 'CREATE TABLE', path: '$CWD/backend' } },
              { type: 'tool_use', name: 'StructuredOutput', input: { ok: true } },
            ],
          },
        },
        {
          type: 'result',
          subtype: 'success',
          session_id: 's1',
          total_cost_usd: 0.25,
          structured_output: { ok: true },
        },
      ]),
      onEvent: (e) => events.push(e),
    });

    expect(result).toEqual({ output: { ok: true }, sessionId: 's1', costUsd: 0.25 });
    expect(events).toEqual([
      { type: 'assistant_message', text: 'Looking around' },
      { type: 'tool_call', name: 'Read', kind: 'read', target: 'backend/go.mod' },
      { type: 'tool_call', name: 'Grep', kind: 'search', target: 'CREATE TABLE in backend' },
      { type: 'usage', costUsd: 0.25 },
      { type: 'turn_end' },
    ]);
  });

  it('runs read-only and outside the user config', async () => {
    await runClaudeStructured({
      cwd: dir,
      prompt: 'p',
      jsonSchema: { $schema: 'https://json-schema.org/draft/2020-12/schema', type: 'object' },
      command: fake,
      env: env([{ type: 'result', subtype: 'success', structured_output: {} }]),
      onEvent: () => {},
    });
    const args: string[] = JSON.parse(readFileSync(join(dir, 'args.jsonl'), 'utf8'));
    const denied = args.slice(args.indexOf('--disallowedTools') + 1);
    expect(denied).toEqual(expect.arrayContaining(['Edit', 'Write', 'Bash']));
    expect(args).not.toContain('--dangerously-skip-permissions');
    expect(args[args.indexOf('--setting-sources') + 1]).toBe('project,local');
    expect(JSON.parse(args[args.indexOf('--json-schema') + 1] ?? '')).toEqual({ type: 'object' });
  });

  it('fails with the agent error', async () => {
    await expect(
      runClaudeStructured({
        cwd: dir,
        prompt: 'p',
        jsonSchema: {},
        command: fake,
        env: env([{ type: 'result', subtype: 'error_max_turns', is_error: true, result: 'too many turns' }]),
        onEvent: () => {},
      }),
    ).rejects.toThrow(AgentFailedError);
  });
});
