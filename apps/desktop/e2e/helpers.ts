/// <reference lib="dom" />
import { mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { _electron as electron, type Page } from '@playwright/test';

/** Starts the built app with its own empty data folder. */
export async function launchStudio() {
  const userData = mkdtempSync(join(tmpdir(), 'studio-e2e-'));
  const app = await electron.launch({
    args: ['.'],
    cwd: new URL('..', import.meta.url).pathname,
    env: { ...process.env, STUDIO_USER_DATA: userData },
  });
  return { app, win: await app.firstWindow(), userData };
}

/** Calls a tRPC mutation through the same IPC bridge the UI uses (the native folder picker can't be clicked). */
export async function mutate(win: Page, path: string, input: unknown): Promise<unknown> {
  return win.evaluate(
    async ({ path, input }) => {
      const res = await window.studio.trpc({
        url: `http://studio.local/trpc/${path}?batch=1`,
        method: 'POST',
        headers: [['content-type', 'application/json']],
        body: JSON.stringify({ 0: input }),
      });
      const [first] = JSON.parse(res.body);
      if (first.error) throw new Error(JSON.stringify(first.error));
      return first.result.data;
    },
    { path, input },
  );
}
