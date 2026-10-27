import { contextBridge } from 'electron';
import type { StudioBridge } from './bridge';

const bridge: StudioBridge = {
  platform: process.platform,
  versions: {
    electron: process.versions.electron ?? '',
    chrome: process.versions.chrome ?? '',
    node: process.versions.node,
  },
};

contextBridge.exposeInMainWorld('studio', bridge);
