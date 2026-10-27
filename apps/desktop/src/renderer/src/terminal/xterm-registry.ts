import { FitAddon } from '@xterm/addon-fit';
import { SearchAddon } from '@xterm/addon-search';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import type { PtyData } from '../../../preload/bridge';

/**
 * One xterm per terminal tab, alive for the whole renderer session. A hidden tab's xterm is parked
 * off-screen, not destroyed, so switching tabs or workspaces is instant and keeps the screen as the
 * program left it (the keep-alive idea of emdash `xterm-host.ts`, Apache-2.0). After a window
 * reload it is rebuilt from main's buffer.
 */
export interface XtermEntry {
  term: Terminal;
  fit: FitAddon;
  search: SearchAddon;
  element: HTMLDivElement;
  /** Last output batch written; live batches up to it are already in the snapshot. */
  seq: number;
  ready: Promise<void>;
}

export interface Snapshot {
  data: string;
  seq: number;
  truncated: boolean;
}

const entries = new Map<string, XtermEntry>();
/** Live output that arrives before the snapshot of a new entry is written. */
const early = new Map<string, PtyData[]>();
let parking: HTMLDivElement | null = null;
let listening = false;

function parkingLot(): HTMLDivElement {
  if (!parking) {
    parking = document.createElement('div');
    // Off-screen but laid out: xterm needs real dimensions to measure its characters.
    parking.style.cssText = 'position:fixed;left:-10000px;top:0;width:800px;height:600px;visibility:hidden';
    parking.setAttribute('aria-hidden', 'true');
    document.body.appendChild(parking);
  }
  return parking;
}

function listen(): void {
  if (listening) return;
  listening = true;
  window.studio.pty.onData((chunk) => {
    const queue = early.get(chunk.terminalId);
    if (queue) {
      queue.push(chunk);
      return;
    }
    const entry = entries.get(chunk.terminalId);
    if (entry && chunk.seq > entry.seq) {
      entry.term.write(chunk.data);
      entry.seq = chunk.seq;
    }
  });
}

function cssVar(name: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/** Colors from the app theme, so the terminal follows Light / Dark / Darcula… */
export function terminalTheme() {
  return {
    background: cssVar('--bg'),
    foreground: cssVar('--ink'),
    cursor: cssVar('--accent'),
    cursorAccent: cssVar('--bg'),
    selectionBackground: cssVar('--sel'),
  };
}

const mac = () => window.studio.platform === 'darwin';

/** Cmd+⌫ deletes the line, Cmd+←/→ go to its start / end, Option+←/→ move by word, as in Terminal.app. */
const CMD_KEYS: Record<string, string> = { Backspace: '\x15', ArrowLeft: '\x01', ArrowRight: '\x05' };
const OPTION_KEYS: Record<string, string> = { ArrowLeft: '\x1bb', ArrowRight: '\x1bf' };

/**
 * Keys a macOS terminal user expects (emdash `pty-keybindings.ts`, idea). Shift+Enter sends a line
 * feed, which Claude Code takes as a new line in its prompt instead of sending it.
 */
function keyHandler(terminalId: string, term: Terminal, onSearch: () => void) {
  const send = (data: string) => window.studio.pty.write(terminalId, data);
  return (e: KeyboardEvent): boolean => {
    if (e.type !== 'keydown') return true;
    const cmd = mac() ? e.metaKey : e.ctrlKey && e.shiftKey;
    if (e.key === 'Enter' && e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey) {
      send('\n');
      return false;
    }
    if (cmd && e.key.toLowerCase() === 'f') {
      onSearch();
      return false;
    }
    if (cmd && e.key.toLowerCase() === 'k') {
      term.clear();
      return false;
    }
    const mapped = mac()
      ? e.metaKey
        ? CMD_KEYS[e.key]
        : e.altKey
          ? OPTION_KEYS[e.key]
          : undefined
      : undefined;
    if (mapped) {
      send(mapped);
      return false;
    }
    return true;
  };
}

/** The xterm of a tab, created on first use and filled from main's buffer. */
export function xtermFor(
  terminalId: string,
  opts: { loadSnapshot: () => Promise<Snapshot>; onSearch: () => void; truncatedNote: string },
): XtermEntry {
  const existing = entries.get(terminalId);
  if (existing) return existing;
  listen();

  const term = new Terminal({
    fontFamily: '"SF Mono", Menlo, Monaco, "JetBrains Mono", monospace',
    fontSize: 12.5,
    lineHeight: 1.15,
    scrollback: 10_000,
    // Claude Code redraws with bare \n; converting it would break its screen.
    convertEol: false,
    allowProposedApi: true,
    macOptionIsMeta: false,
    theme: terminalTheme(),
  });
  const fit = new FitAddon();
  const search = new SearchAddon();
  term.loadAddon(fit);
  term.loadAddon(search);
  // Cmd/Ctrl+click opens a link in the system browser (main turns window.open into openExternal).
  term.loadAddon(
    new WebLinksAddon((event, uri) => {
      if (event.metaKey || event.ctrlKey) window.open(uri);
    }),
  );
  term.attachCustomKeyEventHandler(keyHandler(terminalId, term, opts.onSearch));
  term.onData((data) => window.studio.pty.write(terminalId, data));
  term.onResize(({ cols, rows }) => window.studio.pty.resize(terminalId, cols, rows));

  const element = document.createElement('div');
  element.style.cssText = 'width:100%;height:100%';
  parkingLot().appendChild(element);
  term.open(element);

  early.set(terminalId, []);
  const entry: XtermEntry = { term, fit, search, element, seq: 0, ready: Promise.resolve() };
  entry.ready = opts
    .loadSnapshot()
    .then((snap) => {
      if (snap.truncated) term.write(`\x1b[2m${opts.truncatedNote}\x1b[0m\r\n`);
      term.write(snap.data);
      entry.seq = snap.seq;
    })
    .catch(() => {})
    .finally(() => {
      for (const chunk of early.get(terminalId) ?? []) {
        if (chunk.seq > entry.seq) {
          term.write(chunk.data);
          entry.seq = chunk.seq;
        }
      }
      early.delete(terminalId);
    });
  entries.set(terminalId, entry);
  return entry;
}

/** Shows the tab's xterm in `host`. */
export function attach(entry: XtermEntry, host: HTMLElement): void {
  if (entry.element.parentElement !== host) host.appendChild(entry.element);
  entry.term.options.theme = terminalTheme();
  fitNow(entry);
  // Redraw on the next frame: the canvas was not painted while parked.
  requestAnimationFrame(() => entry.term.refresh(0, entry.term.rows - 1));
}

/** Hides it again without losing anything. */
export function detach(entry: XtermEntry): void {
  parkingLot().appendChild(entry.element);
}

/** Fits the xterm to its container; skipped while the container has no size (hidden). */
export function fitNow(entry: XtermEntry): void {
  const parent = entry.element.parentElement;
  if (!parent || parent === parking || parent.clientWidth === 0 || parent.clientHeight === 0) return;
  try {
    entry.fit.fit();
  } catch {
    // Not measurable yet.
  }
}

/** Forgets a closed tab. */
export function disposeXterm(terminalId: string): void {
  const entry = entries.get(terminalId);
  if (!entry) return;
  entry.term.dispose();
  entry.element.remove();
  entries.delete(terminalId);
  early.delete(terminalId);
}
