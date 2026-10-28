import { appendFileSync, mkdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join } from 'node:path';

/** Longest pain kept; it is a one-line note, not a report. */
export const MAX_PAIN = 2000;

export function painsPath(studioHome: string): string {
  return join(studioHome, 'pains.md');
}

const pad = (n: number) => String(n).padStart(2, '0');

/** `2026-10-27 14:03`, local time: when the developer felt it. */
export function painTime(date: Date): string {
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

/** One Markdown list item: `- 2026-10-27 14:03 · project · workspace · text`; `—` where there is none. */
export function painLine(input: {
  text: string;
  project: string | null;
  workspace: string | null;
  at: Date;
}): string {
  const one = (s: string) => s.replace(/\s+/g, ' ').trim();
  const text = one(input.text).slice(0, MAX_PAIN);
  return `- ${painTime(input.at)} · ${one(input.project ?? '') || '—'} · ${one(input.workspace ?? '') || '—'} · ${text}`;
}

/** Creates an empty `pains.md` if there is none, so it can be opened before the first pain. */
export function ensurePainsFile(studioHome: string): string {
  const path = painsPath(studioHome);
  mkdirSync(dirname(path), { recursive: true });
  appendFileSync(path, '');
  return path;
}

/** Appends the pain to `<studioHome>/pains.md`, creating it; returns the file path. */
export function appendPain(studioHome: string, line: string): string {
  const path = painsPath(studioHome);
  mkdirSync(dirname(path), { recursive: true });
  let prefix = '';
  try {
    // A file edited by hand may not end with a newline; the new item must not stick to the last one.
    if (statSync(path).size > 0 && !readFileSync(path, 'utf8').endsWith('\n')) prefix = '\n';
  } catch {
    // No file yet.
  }
  appendFileSync(path, `${prefix}${line}\n`);
  return path;
}
