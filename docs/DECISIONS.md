# Решения вне спеки

- 2026-10-27: `.npmrc` с публичным `registry.npmjs.org` в корне — глобальный npm-реестр машины корпоративный и недоступен вне VPN; Studio не должна от него зависеть.
- 2026-10-27: без поля `packageManager` в корневом `package.json` — pnpm 12 по нему проверяет свою версию через глобальный реестр и падает без VPN.
- 2026-10-27: сборка разрешена только для `electron`, `better-sqlite3`, `esbuild` (`allowBuilds` в `pnpm-workspace.yaml`, синтаксис pnpm 12).
- 2026-10-27: актуальные мажоры на старте: Electron 45, electron-vite 6, Vite 8, TypeScript 7, Biome 2, vitest 5.
- 2026-10-27: preload собирается в CommonJS (`index.cjs`) — иначе не работает `sandbox: true`.
- 2026-10-27: системный шрифт вместо подключаемого — меньше зависимостей, desktop-приложению не нужен веб-шрифт.
- 2026-10-27: tRPC↔IPC — не `electron-trpc`/`trpc-electron` (последние релизы 12.2024 и 01.2025), а тонкий транспорт на публичном API tRPC v11: `fetchRequestHandler` в main, `fetch` через IPC в renderer, подписки через SSE-линк. Делается в шаге 2 M0.
