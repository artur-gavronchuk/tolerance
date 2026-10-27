import type { StudioEvent } from '@studio/schema';
import { contextBridge, type IpcRendererEvent, ipcRenderer } from 'electron';
import type { StudioBridge } from './bridge';

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
};

contextBridge.exposeInMainWorld('studio', bridge);
