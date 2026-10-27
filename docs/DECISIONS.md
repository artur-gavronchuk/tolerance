# Решения вне спеки

- 2026-10-27: `.npmrc` с публичным `registry.npmjs.org` в корне — глобальный npm-реестр машины корпоративный и недоступен вне VPN; Studio не должна от него зависеть.
- 2026-10-27: без поля `packageManager` в корневом `package.json` — pnpm 12 по нему проверяет свою версию через глобальный реестр и падает без VPN.
- 2026-10-27: сборка разрешена только для `electron`, `better-sqlite3`, `esbuild` (`allowBuilds` в `pnpm-workspace.yaml`, синтаксис pnpm 12).
- 2026-10-27: актуальные мажоры на старте: Electron 45, electron-vite 6, Vite 8, TypeScript 7, Biome 2, vitest 5.
- 2026-10-27: preload собирается в CommonJS (`index.cjs`) — иначе не работает `sandbox: true`.
- 2026-10-27: системный шрифт вместо подключаемого — меньше зависимостей, desktop-приложению не нужен веб-шрифт.
- 2026-10-27: tRPC↔IPC — не `electron-trpc`/`trpc-electron` (последние релизы 12.2024 и 01.2025), а тонкий транспорт на публичном API tRPC v11: `fetchRequestHandler` в main, `fetch` через IPC в renderer, подписки через SSE-линк. Делается в шаге 2 M0.
- 2026-10-27: порядок майлстоунов изменён по просьбе пользователя: сразу после каркаса — «открыть проект → понять, из чего он состоит» (часть M1 + Mapper из M3). Чат с агентом, терминал, OpenCode/Codex — позже.
- 2026-10-27: SQLite — встроенный `node:sqlite` (Node 24 в Electron 45) + drizzle `sqlite-proxy` вместо better-sqlite3: та же схема и миграции drizzle-kit, но без нативного модуля и `electron-rebuild`; тесты идут в обычном Node.
- 2026-10-27: агент для карты — временно Claude Code headless (`claude -p --output-format stream-json --json-schema`, только Read/Glob/Grep, `--setting-sources project,local`, `--strict-mcp-config`), а не ACP. Выход нормализуется в те же `NormalizedEvent`, так что переход на ACP не затронет движок и UI. Структурированный ответ приходит в `structured_output`, а не файлом.
- 2026-10-27: `RepoMapJson` расширен полями компонента `tech` и `entities` (таблицы для db, маршруты для backend) — без них не ответить «что в базе, что на фронте». Все поля обязательные (пустые строки/списки), чтобы схема годилась для строгого structured output.
- 2026-10-27: быстрый скан без LLM (`packages/engine/src/scan.ts`): манифесты, compose-файлы, миграции, CI, тесты, языки по размеру файлов. Даёт картину за десятки миллисекунд, пока агент работает; подписи общих находок — ключи i18n.
- 2026-10-27: обновления UI — не tRPC-подписки, а событие `studio:event` из main (что изменилось) + инвалидация запросов TanStack Query. Проще транспорт (только запрос/ответ через IPC); подписки понадобятся для потока терминала.
- 2026-10-27: git через `node:child_process` `execFile`, без `execa` — одной зависимостью меньше, нужного хватает.
- 2026-10-27: PATH берётся из login-shell при старте main: приложение из Dock/Finder иначе не видит `claude` и Homebrew.
- 2026-10-27: `STUDIO_USER_DATA` переопределяет папку данных — e2e-тесты не трогают базу пользователя.
