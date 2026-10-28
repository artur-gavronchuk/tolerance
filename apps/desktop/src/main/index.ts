import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { openDb } from '@studio/db';
import { Assessor, Implementer, Mapper, Planner, Workspaces } from '@studio/engine';
import { app, BrowserWindow, nativeTheme, shell } from 'electron';
import { startAgentStatus } from './agent-status';
import { broadcast, sendPtyData, servePty, serveTrpc } from './ipc';
import { adoptLoginShellPath } from './shell-path';
import { PtyHost } from './terminals/pty-host';
import { Terminals } from './terminals/terminals';

const here = fileURLToPath(new URL('.', import.meta.url));

// Tests and side-by-side runs keep their database apart from the user's.
if (process.env.STUDIO_USER_DATA) app.setPath('userData', process.env.STUDIO_USER_DATA);

/** e2e runs: the window is never shown, Studio stays out of the Dock and never takes focus. */
const hidden = process.env.STUDIO_HIDDEN_WINDOW === '1';
if (hidden) app.dock?.hide();

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
      // A hidden test window must keep rendering and running timers.
      backgroundThrottling: !hidden,
    },
  });

  if (!hidden) win.once('ready-to-show', () => win.show());

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
  await db.markInterruptedSetups();
  const mapper = new Mapper({ db, notify: broadcast });
  const assessor = new Assessor({ db, notify: broadcast });
  const planner = new Planner({ db, notify: broadcast });
  const implementer = new Implementer({ db, notify: broadcast });
  const workspaces = new Workspaces({ db, notify: broadcast });
  const agentStatus = await startAgentStatus({
    notify: broadcast,
    // The dock shows how many workspaces wait for the developer.
    setBadge: (count) => app.setBadgeCount(count),
    onSession: (terminalId, sessionId) => {
      void db.getTerminal(terminalId).then((terminal) => {
        if (terminal && terminal.sessionId !== sessionId) void db.setTerminalSession(terminalId, sessionId);
      });
    },
  });
  const { statuses } = agentStatus;
  const host = new PtyHost({
    data: (chunk) => {
      sendPtyData(chunk);
      statuses.statuses.output(chunk.terminalId);
    },
    exit: (terminalId) => {
      statuses.statuses.exited(terminalId);
      void db.getTerminal(terminalId).then((terminal) => {
        if (terminal) broadcast({ type: 'terminal', workspaceId: terminal.workspaceId, terminalId });
      });
    },
  });
  const terminals = new Terminals({ db, host, notify: broadcast, hooks: agentStatus.hooks });
  app.on('will-quit', () => {
    workspaces.killAll();
    host.killAll();
    agentStatus.stop();
  });
  servePty(host);
  serveTrpc(() => ({ db, mapper, assessor, planner, implementer, workspaces, terminals, statuses }));

  createWindow();
  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit();
});
