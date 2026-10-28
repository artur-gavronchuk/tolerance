import { randomBytes, timingSafeEqual } from 'node:crypto';
import { mkdirSync, writeFileSync } from 'node:fs';
import { createServer, type Server } from 'node:http';
import { join } from 'node:path';

/** Claude Code hook input: the fields Studio reads. */
export interface HookPayload {
  hook_event_name?: string;
  session_id?: string;
  notification_type?: string;
  message?: string;
}

const BODY_LIMIT = 1024 * 1024;

/**
 * A local HTTP endpoint for Claude Code hooks (the scheme of emdash `hook-server.ts`, Apache-2.0,
 * rewritten). Bound to 127.0.0.1 on a free port. Every agent terminal gets its own token in its
 * environment; the token says which terminal a hook came from, and a request without a known token
 * is refused.
 */
export class HookServer {
  private server: Server | null = null;
  private readonly tokens = new Map<string, string>(); // token → terminal id
  port: number | null = null;

  constructor(private readonly onEvent: (terminalId: string, payload: HookPayload) => void) {}

  async start(): Promise<number> {
    const server = createServer((req, res) => {
      const terminalId = this.terminalFor(req.headers.authorization);
      if (req.method !== 'POST' || req.url !== '/hook') {
        res.writeHead(404).end();
        return;
      }
      if (!terminalId) {
        res.writeHead(403).end();
        return;
      }
      let size = 0;
      const parts: Buffer[] = [];
      req.on('data', (chunk: Buffer) => {
        size += chunk.length;
        if (size > BODY_LIMIT) req.destroy();
        else parts.push(chunk);
      });
      req.on('end', () => {
        let payload: HookPayload;
        try {
          payload = JSON.parse(Buffer.concat(parts).toString('utf8')) as HookPayload;
        } catch {
          res.writeHead(400).end();
          return;
        }
        // An empty object is "no decision": Claude carries on as without the hook.
        res.writeHead(200, { 'content-type': 'application/json' }).end('{}');
        this.onEvent(terminalId, payload);
      });
    });
    await new Promise<void>((resolve, reject) => {
      server.once('error', reject);
      server.listen(0, '127.0.0.1', () => resolve());
    });
    this.server = server;
    const address = server.address();
    this.port = typeof address === 'object' && address ? address.port : null;
    if (this.port === null) throw new Error('Hook server has no port');
    return this.port;
  }

  /** A fresh token for a terminal start; the previous one of that terminal stops working. */
  tokenFor(terminalId: string): string {
    this.revoke(terminalId);
    const token = randomBytes(24).toString('hex');
    this.tokens.set(token, terminalId);
    return token;
  }

  revoke(terminalId: string): void {
    for (const [token, id] of this.tokens) if (id === terminalId) this.tokens.delete(token);
  }

  close(): void {
    this.server?.close();
    this.server = null;
  }

  private terminalFor(header: string | undefined): string | undefined {
    const given = /^Bearer (\S+)$/.exec(header ?? '')?.[1];
    if (!given) return undefined;
    for (const [token, id] of this.tokens) {
      const a = Buffer.from(token);
      const b = Buffer.from(given);
      if (a.length === b.length && timingSafeEqual(a, b)) return id;
    }
    return undefined;
  }
}

/** Hook events Studio listens to; see `AgentStatuses` for what each one means. */
export const HOOK_EVENTS = ['UserPromptSubmit', 'PostToolUse', 'PermissionRequest', 'Notification', 'Stop'];

/** The env variable a hook reads its token from (`allowedEnvVars`, so nothing else leaks into headers). */
export const HOOK_TOKEN_ENV = 'STUDIO_HOOK_TOKEN';

/**
 * Writes the settings file passed as `claude --settings <file>`: only hooks, as HTTP hooks to the
 * local server. The user's own `~/.claude/settings.json` is never touched; Claude adds these hooks
 * on top of the user's.
 */
export function writeClaudeHookSettings(dir: string, port: number): string {
  const hook = {
    type: 'http',
    url: `http://127.0.0.1:${port}/hook`,
    headers: { Authorization: `Bearer $${HOOK_TOKEN_ENV}` },
    allowedEnvVars: [HOOK_TOKEN_ENV],
    timeout: 5,
  };
  const settings = { hooks: Object.fromEntries(HOOK_EVENTS.map((event) => [event, [{ hooks: [hook] }]])) };
  mkdirSync(dir, { recursive: true });
  const file = join(dir, `claude-hooks-${port}.json`);
  writeFileSync(file, `${JSON.stringify(settings, null, 2)}\n`, { mode: 0o600 });
  return file;
}
