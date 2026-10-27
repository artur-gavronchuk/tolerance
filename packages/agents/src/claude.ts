import { spawn } from 'node:child_process';
import { isAbsolute, relative } from 'node:path';
import { createInterface } from 'node:readline';
import type { NormalizedEvent, ToolKind } from '@studio/schema';

/**
 * Claude Code in headless mode (`claude -p --output-format stream-json`), read-only.
 *
 * Temporary until the ACP adapter (SPEC §3.3) lands: same NormalizedEvent output, so the engine
 * and UI will not change when the transport does. See docs/DECISIONS.md.
 */
export interface StructuredRunOptions {
  cwd: string;
  prompt: string;
  /** JSON Schema the final answer must match. */
  jsonSchema: object;
  model?: string;
  /** Continue an earlier session (used to send validation errors back to the agent). */
  resumeSessionId?: string;
  onEvent: (event: NormalizedEvent) => void;
  signal?: AbortSignal;
  /** Executable to run; tests point it at a fake agent. */
  command?: string;
  env?: NodeJS.ProcessEnv;
}

export interface StructuredRunResult {
  output: unknown;
  sessionId: string | null;
  costUsd: number;
}

const READ_ONLY_TOOLS = ['Read', 'Glob', 'Grep', 'LS'];
const DENIED_TOOLS = ['Edit', 'MultiEdit', 'Write', 'NotebookEdit', 'Bash', 'WebFetch', 'WebSearch', 'Task'];

export function claudeReadOnlyArgs(
  opts: Pick<StructuredRunOptions, 'prompt' | 'jsonSchema' | 'model' | 'resumeSessionId'>,
): string[] {
  return [
    '-p',
    opts.prompt,
    '--output-format',
    'stream-json',
    '--verbose',
    '--json-schema',
    JSON.stringify(withoutMetaSchema(opts.jsonSchema)),
    '--model',
    opts.model ?? 'sonnet',
    '--allowedTools',
    ...READ_ONLY_TOOLS,
    '--disallowedTools',
    ...DENIED_TOOLS,
    // The user's own hooks, plugins and MCP servers stay out of Studio runs; their global config is never touched.
    '--setting-sources',
    'project,local',
    '--strict-mcp-config',
    ...(opts.resumeSessionId ? ['--resume', opts.resumeSessionId] : []),
  ];
}

/** Claude Code rejects a `$schema` pointing at a draft it doesn't bundle (zod emits 2020-12); the shape is all it needs. */
function withoutMetaSchema(schema: object): object {
  const { $schema: _ignored, ...rest } = schema as { $schema?: unknown };
  return rest;
}

const ANSI = new RegExp(`${String.fromCharCode(27)}\\[[0-9;]*m`, 'g');

export async function runClaudeStructured(opts: StructuredRunOptions): Promise<StructuredRunResult> {
  const child = spawn(opts.command ?? 'claude', claudeReadOnlyArgs(opts), {
    cwd: opts.cwd,
    env: opts.env ?? process.env,
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  const abort = () => child.kill('SIGTERM');
  opts.signal?.addEventListener('abort', abort, { once: true });

  let stderr = '';
  child.stderr.on('data', (chunk) => {
    stderr = (stderr + chunk).replace(ANSI, '').slice(-4000);
  });

  const normalizer = new ClaudeStreamNormalizer(opts.cwd);
  let result: StructuredRunResult | null = null;
  let failure: string | null = null;

  for await (const line of createInterface({ input: child.stdout })) {
    if (!line.trim()) continue;
    let message: ClaudeStreamMessage;
    try {
      message = JSON.parse(line);
    } catch {
      continue;
    }
    for (const event of normalizer.push(message)) opts.onEvent(event);
    if (message.type === 'result') {
      if (message.is_error || message.subtype !== 'success') {
        failure = message.result || message.subtype || 'agent reported an error';
      } else {
        result = {
          output: message.structured_output ?? null,
          sessionId: message.session_id ?? null,
          costUsd: message.total_cost_usd ?? 0,
        };
      }
    }
  }

  const exitCode: number | null = await new Promise((resolve) => {
    if (child.exitCode !== null || child.signalCode !== null) resolve(child.exitCode);
    else child.once('close', (code) => resolve(code));
  });
  opts.signal?.removeEventListener('abort', abort);

  if (opts.signal?.aborted) throw new AgentCancelledError();
  if (result) return result;
  throw new AgentFailedError(
    failure ?? `claude exited with code ${exitCode}: ${stderr.trim() || 'no output'}`,
  );
}

export class AgentCancelledError extends Error {
  constructor() {
    super('Agent run cancelled');
  }
}

export class AgentFailedError extends Error {}

// ── stream-json → NormalizedEvent ───────────────────────────────────────────

type ContentBlock =
  | { type: 'text'; text: string }
  | { type: 'thinking'; thinking: string }
  | { type: 'tool_use'; name: string; input: Record<string, unknown> }
  | { type: string };

export interface ClaudeStreamMessage {
  type: string;
  subtype?: string;
  message?: { content?: ContentBlock[] | string };
  is_error?: boolean;
  result?: string;
  structured_output?: unknown;
  session_id?: string;
  total_cost_usd?: number;
}

const TOOL_KINDS: Record<string, ToolKind> = {
  Read: 'read',
  LS: 'read',
  NotebookRead: 'read',
  Glob: 'search',
  Grep: 'search',
  Edit: 'edit',
  MultiEdit: 'edit',
  Write: 'edit',
  NotebookEdit: 'edit',
  Bash: 'command',
  WebFetch: 'fetch',
  WebSearch: 'fetch',
};

export class ClaudeStreamNormalizer {
  constructor(private readonly cwd: string) {}

  push(message: ClaudeStreamMessage): NormalizedEvent[] {
    if (message.type === 'assistant' && Array.isArray(message.message?.content)) {
      return message.message.content.flatMap((block) => this.block(block));
    }
    if (message.type === 'result') {
      const events: NormalizedEvent[] = [];
      if (typeof message.total_cost_usd === 'number')
        events.push({ type: 'usage', costUsd: message.total_cost_usd });
      if (message.is_error || message.subtype !== 'success') {
        events.push({ type: 'error', message: message.result || message.subtype || 'error' });
      }
      events.push({ type: 'turn_end' });
      return events;
    }
    return [];
  }

  private block(block: ContentBlock): NormalizedEvent[] {
    if (block.type === 'text' && 'text' in block && block.text.trim()) {
      return [{ type: 'assistant_message', text: block.text }];
    }
    if (block.type === 'thinking' && 'thinking' in block && block.thinking.trim()) {
      return [{ type: 'thinking', text: block.thinking }];
    }
    if (block.type === 'tool_use' && 'name' in block) {
      // The final structured answer arrives as a tool call; it is the result, not progress.
      if (block.name === 'StructuredOutput') return [];
      return [
        {
          type: 'tool_call',
          name: block.name,
          kind: TOOL_KINDS[block.name] ?? 'other',
          target: this.target(block.input),
        },
      ];
    }
    return [];
  }

  private target(input: Record<string, unknown>): string {
    const str = (key: string) => {
      const value = input[key];
      return typeof value === 'string' ? value : '';
    };
    const path = this.rel(str('file_path') || str('notebook_path') || str('path'));
    const pattern = str('pattern');
    if (pattern) return path ? `${pattern} in ${path}` : pattern;
    return path || str('command') || str('url') || str('query') || str('description');
  }

  private rel(path: string): string {
    if (!path || !isAbsolute(path)) return path;
    const r = relative(this.cwd, path);
    return r.startsWith('..') ? path : r || '.';
  }
}

/** Whether the `claude` CLI is on PATH, and its version. */
export async function detectClaude(
  command = 'claude',
): Promise<{ installed: boolean; version: string | null }> {
  const { execFile } = await import('node:child_process');
  return new Promise((resolve) => {
    execFile(command, ['--version'], { timeout: 10_000 }, (error, stdout) => {
      if (error) resolve({ installed: false, version: null });
      else resolve({ installed: true, version: stdout.trim().split(/\s+/)[0] ?? null });
    });
  });
}
