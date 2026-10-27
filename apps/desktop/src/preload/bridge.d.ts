import type { StudioEvent } from '@studio/schema';

/** A batch of terminal output from main; `seq` grows by one per batch of that terminal. */
export interface PtyData {
  terminalId: string;
  seq: number;
  data: string;
}

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
  /**
   * Terminal I/O. It bypasses tRPC: keystrokes and output are many small messages, and output is
   * pushed from main. Starting, listing and snapshots go through tRPC.
   */
  pty: {
    onData(listener: (data: PtyData) => void): () => void;
    write(terminalId: string, data: string): void;
    resize(terminalId: string, cols: number, rows: number): void;
  };
}

declare global {
  interface Window {
    studio: StudioBridge;
  }
}
