import type { StudioEvent } from '@studio/schema';

export interface IpcRequest {
  url: string;
  method: string;
  headers: [string, string][];
  body: string | null;
}

export interface IpcResponse {
  status: number;
  headers: [string, string][];
  body: string;
}

/** What the preload script exposes to the renderer as `window.studio`. */
export interface StudioBridge {
  platform: string;
  versions: { electron: string; chrome: string; node: string };
  /** Transport for the tRPC client (see src/main/ipc.ts). */
  trpc(request: IpcRequest): Promise<IpcResponse>;
  /** Subscribes to change notifications from main; returns an unsubscribe function. */
  onEvent(listener: (event: StudioEvent) => void): () => void;
}

declare global {
  interface Window {
    studio: StudioBridge;
  }
}
