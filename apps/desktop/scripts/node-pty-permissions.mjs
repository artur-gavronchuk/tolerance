// node-pty ships its macOS `spawn-helper` without the executable bit; every spawn then fails with
// "posix_spawnp failed". Restores it after install (same fix as emdash `scripts/postinstall.ts`, Apache-2.0).
import { chmodSync, existsSync, readdirSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';

const require = createRequire(import.meta.url);
let root;
try {
  root = dirname(require.resolve('node-pty/package.json'));
} catch {
  process.exit(0);
}
const prebuilds = join(root, 'prebuilds');
if (existsSync(prebuilds)) {
  for (const dir of readdirSync(prebuilds)) {
    const helper = join(prebuilds, dir, 'spawn-helper');
    if (dir.startsWith('darwin-') && existsSync(helper)) chmodSync(helper, 0o755);
  }
}
