import type { StudioEvent } from '@studio/schema';
import { contextBridge, type IpcRendererEvent, ipcRenderer } from 'electron';
import type { PtyData, StudioBridge } from './bridge';

const bridge: StudioBridge = {
  platform: process.platform,
  versions: {
    electron: process.versions.electron ?? '',
    chrome: process.versions.chrome ?? '',
    node: process.versions.node,
  },
  trpc: (request) => ipcRenderer.invoke('studio:trpc', request),
  onEvent: (listener) => {
    const handler = (_: IpcRendererEvent, event: StudioEvent) => listener(event);
    ipcRenderer.on('studio:event', handler);
    return () => ipcRenderer.removeListener('studio:event', handler);
  },
  pty: {
    onData: (listener) => {
      const handler = (_: IpcRendererEvent, data: PtyData) => listener(data);
      ipcRenderer.on('studio:pty:data', handler);
      return () => ipcRenderer.removeListener('studio:pty:data', handler);
    },
    write: (terminalId, data) => ipcRenderer.send('studio:pty:input', terminalId, data),
    resize: (terminalId, cols, rows) => ipcRenderer.send('studio:pty:resize', terminalId, cols, rows),
  },
};

contextBridge.exposeInMainWorld('studio', bridge);
