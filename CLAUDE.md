# CLAUDE.md — Studio

Desktop-приложение: план изменения → параллельные агенты в git worktree → проверка результата против плана.
Полная спецификация: `docs/SPEC.md`. Читай её перед любой работой.

## Команды

- `pnpm dev` — запуск Electron в dev
- `pnpm check` — typecheck + lint (Biome) + unit-тесты. Должен быть зелёным перед каждым коммитом
- `pnpm test:e2e` — Playwright smoke
- `pnpm db:generate` — миграции drizzle

## Структура

- `apps/desktop` — Electron (main / preload / renderer)
- `packages/schema` — zod-схемы, единственный источник типов
- `packages/db` — drizzle
- `packages/agents` — ACP-клиент, claude / codex / opencode, нормализация событий
- `packages/git` — worktree, diff, merge
- `packages/engine` — mapper, planner, scheduler, integrator, verifier
- `packages/prompts` — шаблоны промптов (.md)

## Жёсткие правила

- Работаем по майлстоунам из SPEC §5, по порядку. Итог майлстоуна — `docs/milestones/Mx.md`.
- Решения вне спеки — одной строкой в `docs/DECISIONS.md`.
- Не писать в глобальные конфиги агентов (`~/.claude`, `~/.codex`, `~/.config/opencode`).
- Не использовать bypass-permissions по умолчанию.
- Не мержить в ветки пользователя; результат — интеграционная ветка `studio/*`.
- `rm`/`worktree remove` — только для путей внутри `~/.studio/worktrees` после realpath.
- Код из Superset (ELv2), Maestro, Claude Squad (AGPL), cmux (GPL) не копировать. Из MIT/Apache — только с атрибуцией в `THIRD_PARTY_NOTICES.md`.
- Все строки UI — через i18next (`ru` по умолчанию, `en`).
- Тесты агентов — через fake ACP agent, не через реальные CLI.
