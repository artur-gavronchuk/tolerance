import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { openDb } from '@studio/db';
import { Mapper } from '@studio/engine';
import { app, BrowserWindow, nativeTheme, shell } from 'electron';
import { broadcast, serveTrpc } from './ipc';
import { adoptLoginShellPath } from './shell-path';

const here = fileURLToPath(new URL('.', import.meta.url));

// Tests and side-by-side runs keep their database apart from the user's.
if (process.env.STUDIO_USER_DATA) app.setPath('userData', process.env.STUDIO_USER_DATA);

function createWindow(): void {
  const win = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 900,
    minHeight: 600,
    show: false,
    title: 'Studio',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    backgroundColor: nativeTheme.shouldUseDarkColors ? '#1e1f22' : '#ffffff',
    webPreferences: {
      preload: join(here, '../preload/index.cjs'),
      contextIsolation: true,
      sandbox: true,
      nodeIntegration: false,
    },
  });

  win.once('ready-to-show', () => win.show());

  // Links open in the system browser, never inside the app window.
  win.webContents.setWindowOpenHandler(({ url }) => {
    if (url.startsWith('https://') || url.startsWith('http://')) void shell.openExternal(url);
    return { action: 'deny' };
  });

  const devUrl = process.env.ELECTRON_RENDERER_URL;
  if (!app.isPackaged && devUrl) void win.loadURL(devUrl);
  else void win.loadFile(join(here, '../renderer/index.html'));
}

/** Migrations ship next to the app in packaged builds (M6); in dev they are read from the db package. */
function migrationsFolder(): string {
  return app.isPackaged
    ? join(process.resourcesPath, 'migrations')
    : join(app.getAppPath(), '../../packages/db/drizzle');
}

app.whenReady().then(async () => {
  await adoptLoginShellPath();
  const db = await openDb(join(app.getPath('userData'), 'studio.db'), migrationsFolder());
  await db.markInterruptedRuns();
  const mapper = new Mapper({ db, notify: broadcast });
  serveTrpc(() => ({ db, mapper }));

  createWindow();
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});
