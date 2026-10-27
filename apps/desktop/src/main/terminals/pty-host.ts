import type { TerminalProcess } from '@studio/schema';
import { type IPty, spawn } from 'node-pty';

/** Output kept per terminal for re-attaching (emdash `wire/src/live/log/source.ts`, idea; Apache-2.0). */
const BUFFER_LIMIT = 1024 * 1024;
/** Output is sent to the UI in batches at most this often, so a chatty process does not flood IPC. */
const BATCH_MS = 8;

export interface PtyChunk {
  terminalId: string;
  /** Increases by one per batch; lets the UI drop what its snapshot already had. */
  seq: number;
  data: string;
}

export interface PtySnapshot {
  data: string;
  seq: number;
  /** Older output was dropped to stay within the buffer limit. */
  truncated: boolean;
  process: TerminalProcess;
}

interface Session {
  pty: IPty | null;
  chunks: { seq: number; data: string }[];
  size: number;
  seq: number;
  truncated: boolean;
  pending: string;
  timer: ReturnType<typeof setTimeout> | null;
  process: TerminalProcess;
}

export interface PtyHostEvents {
  data: (chunk: PtyChunk) => void;
  exit: (terminalId: string, process: TerminalProcess) => void;
}

/**
 * The pseudo-terminals of all workspaces, in the main process. Output is buffered here, so a tab can
 * be hidden, shown again or the window reloaded without losing what the program printed. Processes
 * end with the app: no daemon (SPEC §3).
 */
export class PtyHost {
  private readonly sessions = new Map<string, Session>();

  constructor(private readonly events: PtyHostEvents) {}

  process(terminalId: string): TerminalProcess {
    return this.sessions.get(terminalId)?.process ?? { state: 'never', exitCode: null };
  }

  /** Starts a program in a new terminal, replacing a finished one with the same id. */
  start(
    terminalId: string,
    opts: { file: string; args: string[]; cwd: string; env: NodeJS.ProcessEnv; cols: number; rows: number },
  ): void {
    const existing = this.sessions.get(terminalId);
    if (existing?.process.state === 'running') throw new Error('The terminal is already running');
    const session: Session = existing ?? {
      pty: null,
      chunks: [],
      size: 0,
      seq: 0,
      truncated: false,
      pending: '',
      timer: null,
      process: { state: 'never', exitCode: null },
    };
    const pty = spawn(opts.file, opts.args, {
      name: 'xterm-256color',
      cols: clampCols(opts.cols),
      rows: clampRows(opts.rows),
      cwd: opts.cwd,
      env: opts.env as Record<string, string>,
    });
    session.pty = pty;
    session.process = { state: 'running', exitCode: null };
    this.sessions.set(terminalId, session);

    pty.onData((data) => {
      session.pending += data;
      session.timer ??= setTimeout(() => this.flush(terminalId, session), BATCH_MS);
    });
    pty.onExit(({ exitCode }) => {
      this.flush(terminalId, session);
      if (session.pty !== pty) return; // Restarted meanwhile.
      session.pty = null;
      session.process = { state: 'exited', exitCode };
      this.events.exit(terminalId, session.process);
    });
  }

  write(terminalId: string, data: string): void {
    this.sessions.get(terminalId)?.pty?.write(data);
  }

  resize(terminalId: string, cols: number, rows: number): void {
    try {
      this.sessions.get(terminalId)?.pty?.resize(clampCols(cols), clampRows(rows));
    } catch {
      // The process just exited; its pty is gone (EBADF / ENOTTY).
    }
  }

  /** Everything still buffered, up to the last batch already sent as live data. */
  snapshot(terminalId: string): PtySnapshot {
    const session = this.sessions.get(terminalId);
    if (!session) return { data: '', seq: 0, truncated: false, process: this.process(terminalId) };
    this.flush(terminalId, session);
    return {
      data: session.chunks.map((c) => c.data).join(''),
      seq: session.seq,
      truncated: session.truncated,
      process: session.process,
    };
  }

  /** Hangs up the program (SIGHUP to its process group), then kills what is left after 2 seconds. */
  kill(terminalId: string): void {
    const pty = this.sessions.get(terminalId)?.pty;
    if (pty) hangUp(pty);
  }

  /** Stops and forgets a terminal, e.g. when its tab is closed. */
  dispose(terminalId: string): void {
    this.kill(terminalId);
    const session = this.sessions.get(terminalId);
    if (session?.timer) clearTimeout(session.timer);
    this.sessions.delete(terminalId);
  }

  /** On app quit: programs do not outlive Studio. */
  killAll(): void {
    for (const session of this.sessions.values()) {
      if (session.pty) signalGroup(session.pty, 'SIGHUP');
    }
  }

  private flush(terminalId: string, session: Session): void {
    if (session.timer) clearTimeout(session.timer);
    session.timer = null;
    if (!session.pending) return;
    const data = session.pending;
    session.pending = '';
    session.seq += 1;
    session.chunks.push({ seq: session.seq, data });
    session.size += data.length;
    // Whole chunks are dropped from the front, so escape sequences are not cut in half; the last stays.
    while (session.size > BUFFER_LIMIT && session.chunks.length > 1) {
      session.size -= session.chunks.shift()?.data.length ?? 0;
      session.truncated = true;
    }
    this.events.data({ terminalId, seq: session.seq, data });
  }
}

const clampCols = (cols: number) => Math.max(2, Math.floor(cols) || 80);
const clampRows = (rows: number) => Math.max(1, Math.floor(rows) || 24);

function signalGroup(pty: IPty, signal: NodeJS.Signals): void {
  try {
    // The pty child leads its own session and process group, so -pid reaches everything it started.
    process.kill(-pty.pid, signal);
  } catch {
    try {
      pty.kill(signal);
    } catch {
      // Already gone.
    }
  }
}

function hangUp(pty: IPty): void {
  signalGroup(pty, 'SIGHUP');
  let exited = false;
  pty.onExit(() => {
    exited = true;
  });
  setTimeout(() => {
    if (!exited) signalGroup(pty, 'SIGKILL');
  }, 2000).unref();
}
