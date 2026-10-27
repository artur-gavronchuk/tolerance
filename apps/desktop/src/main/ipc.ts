import type { StudioEvent } from '@studio/schema';
import { fetchRequestHandler } from '@trpc/server/adapters/fetch';
import { BrowserWindow, ipcMain } from 'electron';
import type { IpcRequest, IpcResponse } from '../preload/bridge';
import { type Context, router } from './router';

/**
 * tRPC over Electron IPC: the renderer's tRPC client sends its HTTP-shaped request through IPC and
 * main answers it with tRPC's own fetch adapter. Only public tRPC APIs; see docs/DECISIONS.md.
 */
export function serveTrpc(createContext: () => Context): void {
  ipcMain.handle('studio:trpc', async (_event, req: IpcRequest): Promise<IpcResponse> => {
    const res = await fetchRequestHandler({
      endpoint: '/trpc',
      req: new Request(req.url, {
        method: req.method,
        headers: req.headers,
        body: req.method === 'GET' || req.method === 'HEAD' ? undefined : req.body,
      }),
      router,
      createContext,
    });
    return { status: res.status, headers: [...res.headers.entries()], body: await res.text() };
  });
}

/** Tells every window that something changed; the renderer refetches the affected queries. */
export function broadcast(event: StudioEvent): void {
  for (const win of BrowserWindow.getAllWindows()) win.webContents.send('studio:event', event);
}
