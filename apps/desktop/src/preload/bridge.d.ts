/** What the preload script exposes to the renderer as `window.studio`. */
export interface StudioBridge {
  platform: string;
  versions: { electron: string; chrome: string; node: string };
}

declare global {
  interface Window {
    studio: StudioBridge;
  }
}
