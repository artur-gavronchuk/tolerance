# Studio: спецификация v0.1

> Рабочее название: **Studio** (Visual Agent Studio). Desktop-приложение, в котором разработчик сначала утверждает **план изменения**, затем агенты параллельно реализуют его в git worktree, а система **проверяет результат относительно плана**.
>
> Документ для исполнителя (Claude Opus в Claude Code). Читать целиком перед первым коммитом. Если что-то в коде противоречит спеке, побеждает спека; если спека неполна, принять разумное решение и записать его в `docs/DECISIONS.md`.

---

## 0. Позиционирование в одном абзаце

Superset, Emdash, Conductor, Maestro управляют **сессиями агентов**: терминалы, worktree, diff. Слой сессий стал commodity: у Emdash 37 агентов, у Superset 22. Мы не соревнуемся в количестве агентов. Наш объект управления — **изменение продукта**:

**Запрос → варианты плана → утверждённый план → параллельное исполнение → проверка «сделано ли то, что утвердили» → один итоговый diff.**

Ближайший аналог — Automaker (spec → задачи → граф зависимостей), но он заброшен (последний коммит 2026-05), не делает проверку результата против плана и запускает агентов с `bypassPermissions`. Vibe Kanban объявил sunsetting.

Рынок РФ: первые пользователи — русскоязычные разработчики. Им важны русский UI, работа через прокси и подключение российских и открытых моделей через OpenCode (GigaChat, YandexGPT, DeepSeek, Qwen, Ollama). Это настройки, а не отдельная архитектура.

---

## 1. Цели и не-цели v0.1

### Цели (v0.1 считается готовой, когда всё это работает end-to-end)
1. Открыть локальный git-репозиторий как проект.
2. Построить **карту проекта**: компоненты, связи, ключевые файлы. Генерирует агент, рендерится граф.
3. Ввести запрос на изменение и получить **2 варианта плана** в строгой JSON-схеме. Варианты отличаются подходом, а не формулировками.
4. Увидеть на карте, какие компоненты затрагивает каждый вариант; отредактировать задачи, зависимости и критерии приёмки; **утвердить** один вариант.
5. **Исполнить** план: задачи идут в топологическом порядке, независимые — параллельно (лимит `maxConcurrency`), каждая в своём worktree и со своим агентом. Прогресс виден в реальном времени.
6. **Интегрировать** ветки задач в интеграционную ветку плана. При конфликте запускается агент разрешения конфликта или задача помечается для ручного разбора.
7. **Проверить**: прогнать тесты проекта, сделать детерминированную проверку scope и запустить агента-верификатора по критериям приёмки. На выходе отчёт: по каждому критерию pass / fail / unclear со ссылками на file:line.
8. **Ревью**: diff по задаче и общий diff плана; инлайн-комментарии отправляются агенту как follow-up.
9. Терминал в любом worktree для ручной работы.
10. Агенты: **Claude Code, Codex, OpenCode** через единый ACP-адаптер.

### Не-цели v0.1 (не делать, даже если «быстро»)
- Вкладки Design, Database, Deploy.
- Telegram, мобильное приложение, удалённый доступ, облако, аккаунты, оплата.
- Собственный агентный движок или прямые вызовы LLM API из приложения. Исключение — нет; всё идёт через агентов.
- Поддержка больше трёх агентов. Архитектура должна позволять добавить агента одним файлом, но добавлять не нужно.
- Agent Time Machine, хранение истории сверх того, что даёт git и наш журнал событий.
- Windows (только macOS и Linux).

### Демо-сценарий (acceptance всего v0.1)
На репозитории `github.com/tolerance-dev/arena`: запрос «добавить голосование и рейтинг решений». Ожидаемый результат: план из 2 вариантов, утверждение одного, 3–6 задач, исполненных минимум двумя параллельными агентами, интеграционная ветка, отчёт верификации, общий diff. Время на всё без учёта работы агентов — не больше 10 минут кликов.

---

## 2. Стек

| Слой | Выбор | Почему |
|---|---|---|
| Оболочка | **Electron** (последний стабильный) + electron-vite | Все рабочие аналоги (Superset, Emdash, Maestro, Sculptor) на Electron: node-pty, ACP-SDK, git-процессы живут в Node |
| Язык | **TypeScript strict** везде | Один язык; ACP SDK и Claude Agent SDK на TS |
| Monorepo | **pnpm workspaces** (без Nx/Turbo на старте) | Минимум инфраструктуры |
| UI | React 19, Tailwind 4, shadcn/ui, zustand, TanStack Query | Стандарт, хорошо пишется агентами |
| Граф | **@xyflow/react** + **@dagrejs/dagre** (layout) | Карта проекта и граф задач |
| Diff | **Monaco DiffEditor** (`@monaco-editor/react`) | MIT, side-by-side и inline, так делает Emdash |
| Терминал | **xterm.js** + addons fit, webgl, search; **node-pty** в main | Стандарт |
| БД | **better-sqlite3 + drizzle-orm** (актуальная версия), миграции drizzle-kit | Так делают Superset и Emdash |
| Схемы | **zod** — единственный источник типов для IPC, планов и событий | Валидация вывода агентов |
| IPC | **tRPC поверх Electron IPC** (`electron-trpc` или эквивалент) + подписки для стримов | Типобезопасно, без своего RPC (свой — ошибка Emdash) |
| Агенты | **ACP**: `@agentclientprotocol/sdk` + `@agentclientprotocol/claude-agent-acp`, `@agentclientprotocol/codex-acp`, `opencode acp` | Один протокол на трёх агентов |
| Git | системный `git` через `execa` (не libgit2/isomorphic-git) | Worktree, merge-base, корректная работа с хуками и LFS |
| i18n | **i18next**, языки `ru` (по умолчанию) и `en` с первого дня | Все строки UI только через ключи |
| Тесты | vitest (unit), Playwright для Electron (e2e smoke) | |
| Линт | Biome | Один инструмент вместо eslint+prettier |

**Go не используем.** Вся работа — оркестрация процессов и UI, и Node с этим справляется. Если позже понадобится удалённый воркер-демон, он станет отдельным пакетом.

---

## 3. Архитектура

```
apps/desktop
  src/main/            Electron main: окна, tRPC router, сервисы
  src/preload/
  src/renderer/        React UI
packages/
  schema/              zod-схемы: Project, RepoMap, Plan, Task, Run, AgentEvent, VerifyReport
  db/                  drizzle schema + migrations + repo-функции
  agents/              ACP-клиент, реестр агентов, нормализация событий
  git/                 worktree, branches, diff, merge, conflicts
  engine/              планировщик, исполнитель (scheduler), интегратор, верификатор
  prompts/             шаблоны промптов (md + функции подстановки), версионируются
```

Процессная модель: **один main-процесс** с сервисами; агенты — дочерние процессы (ACP по stdio); терминалы — node-pty. Pty-daemon, переживающий перезапуск (как у Superset), в v0.1 не делаем. При рестарте приложения незавершённые runs помечаются `interrupted` и могут быть возобновлены через ACP session resume, если агент его поддерживает, иначе перезапускаются.

### 3.1 Доменная модель (SQLite)

```ts
Project   { id, name, repoPath, defaultBranch, testCommand?, setupCommand?, createdAt }
RepoMap   { id, projectId, commitSha, json: RepoMapJson, createdAt }          // карта проекта
Plan      { id, projectId, request, status, chosenVariantId?, baseSha,
            integrationBranch, createdAt }
            // status: drafting | awaiting_choice | approved | executing | integrating
            //         | verifying | ready_for_review | done | failed | cancelled
PlanVariant { id, planId, title, json: PlanVariantJson, createdAt }
Task      { id, planId, key /* T1 */, title, status, agentId, branch?, worktreePath?,
            dependsOn: string[] /* task keys */, json: TaskSpecJson }
            // status: pending | blocked | running | awaiting_input | done | failed | cancelled
Run       { id, taskId?, planId, kind, agentId, acpSessionId?, status,
            beforeSha?, afterSha?, startedAt, endedAt?, exitReason? }
            // kind: map | plan | implement | follow_up | resolve_conflict | verify
AgentEvent { id, runId, seq, ts, type, payload }   // нормализованный журнал, append-only
ReviewComment { id, planId, taskId?, file, line, body, sentInRunId? }
VerifyReport { id, planId, json: VerifyReportJson, createdAt }
```

Сырые и нормализованные события агента пишутся в `AgentEvent` (append-only, `seq` монотонный в пределах run) и стримятся в UI подпиской. **Не** парсить markdown-лог обратно, как делает Automaker.

### 3.2 Ключевые JSON-схемы (zod, `packages/schema`)

```ts
RepoMapJson = {
  summary: string,                         // что это за продукт, 2–4 предложения
  components: { id, name, kind: 'frontend'|'backend'|'service'|'library'|'db'|'infra'|'test'|'other',
                paths: string[], description: string }[],
  edges: { from, to, kind: 'calls'|'imports'|'reads'|'writes'|'publishes'|'other', note?: string }[],
  entrypoints: string[],
  testCommandGuess?: string
}

PlanVariantJson = {
  title: string,
  approach: string,                        // суть подхода, 3–6 предложений
  tradeoffs: { pros: string[], cons: string[], risks: string[] },
  complexity: 'S'|'M'|'L',
  affectedComponents: { componentId: string, change: 'add'|'modify'|'remove', note: string }[],
  tasks: TaskSpecJson[]
}

TaskSpecJson = {
  key: string,                              // 'T1'
  title: string,
  description: string,                      // что сделать, достаточно для агента без контекста чата
  dependsOn: string[],
  scope: { allow: string[] /* glob */, deny?: string[] },
  acceptance: { id: string /* 'T1.A1' */, text: string, check: 'test'|'inspect'|'command',
                command?: string }[],
  agentHint?: 'claude'|'codex'|'opencode'
}

VerifyReportJson = {
  tests: { command: string, exitCode: number, summary: string, logTail: string },
  scope: { taskKey: string, outOfScopeFiles: string[] }[],    // детерминированно, без LLM
  criteria: { id: string, verdict: 'pass'|'fail'|'unclear', evidence: { file: string, line?: number, note: string }[] }[],
  unplannedChanges: { file: string, note: string }[],          // изменения, не покрытые ни одной задачей
  overall: 'pass'|'fail'|'needs_review'
}
```

Вывод агента для `map`, `plan`, `verify` парсится и валидируется zod-схемой. Если JSON невалиден, агенту отправляется follow-up с текстом ошибки валидации, максимум 2 повтора, затем run получает статус `failed`. Агент записывает JSON **в файл** (`.studio/out/<runId>.json` внутри worktree или временной директории), а не в чат — так надёжнее.

### 3.3 Агенты (`packages/agents`)

Единый интерфейс поверх ACP:

```ts
interface AgentDefinition {
  id: 'claude' | 'codex' | 'opencode';
  label: string;
  detect(): Promise<{ installed: boolean; version?: string; path?: string }>;
  buildSpawn(ctx: { cwd: string; env: Record<string,string> }): { command: string; args: string[]; env?: Record<string,string> };
  minVersion?: string;
}
interface AgentSession {
  prompt(text: string, opts?: { attachments?: string[] }): Promise<TurnResult>;  // стримит события через onEvent
  cancel(): Promise<void>;
  onEvent(cb: (e: NormalizedEvent) => void): () => void;
  onPermissionRequest(cb: (r: PermissionRequest) => Promise<PermissionDecision>): void;
  sessionId: string;
  dispose(): Promise<void>;
}
```

- Claude: `npx -y @agentclientprotocol/claude-agent-acp@<pinned>`; Codex: `npx -y @agentclientprotocol/codex-acp@<pinned>`; OpenCode: `opencode acp`. **Версии пиннить** в одном файле `packages/agents/src/versions.ts`.
- **NormalizedEvent** (идею взять из Vibe Kanban `NormalizedEntryType`):
  `user_message | assistant_message | thinking | tool_call{name, kind: read|edit|command|search|fetch|other, status, files?} | plan_update | permission_request | usage | error | turn_end`.
- **Права:** у каждого `Run.kind` своя политика. `map`, `plan`, `verify` получают только чтение (edit и command запрещены, кроме записи в `.studio/out/`). `implement`, `follow_up`, `resolve_conflict` разрешают edit внутри worktree и команды из allowlist проекта; всё прочее уходит в UI как запрос подтверждения. **Не использовать** `bypassPermissions` / `--dangerously-skip-permissions` по умолчанию; это только явная опция в настройках проекта.
- **Статус агента** берём из ACP-событий (turn start/end, permission request), а не из парсинга вывода терминала. Добавить watchdog по тишине (идея Maestro): если за N минут (настройка, по умолчанию 10) нет событий, статус `stalled` и уведомление.
- **Модели и провайдеры для РФ:** выбор модели передаётся агенту через его собственные механизмы. OpenCode умеет OpenAI-совместимых провайдеров, поэтому GigaChat, YandexGPT, DeepSeek, Qwen и Ollama настраиваются через его конфиг. В Studio это экран «Провайдеры», который пишет `opencode.json` в **проектный** конфиг, без глобальных правок. Прокси: настройка `HTTPS_PROXY`/`HTTP_PROXY`/`NO_PROXY`, пробрасывается в env агентов.
- **Никогда не писать в глобальные конфиги агентов** (`~/.claude/settings.json` и т.п.). Это слабое место Superset и Emdash, которое они сами задокументировали.

### 3.4 Git (`packages/git`)

- Worktree лежат в `~/.studio/worktrees/<projectSlug>/<planId>/<taskKey>`.
- Ветки: интеграционная `studio/<planShortId>-<slug>`, задачи `studio/<planShortId>/<taskKey>-<slug>`.
- Ветка задачи создаётся от **текущей головы интеграционной ветки** на момент старта задачи, так что задачи-потомки видят результат зависимостей.
- После завершения задачи: если агент не закоммитил, коммитим за него (`studio: <taskKey> <title>`), записываем `afterSha`.
- **Diff** считается от `git merge-base <integrationBranch> <taskBranch>` (так у Superset и Emdash); untracked-файлы включаются.
- **Интеграция:** задачи вливаются в интеграционную ветку по одной, в топологическом порядке, `git merge --no-ff`. Конфликт определяется через `git diff --name-only --diff-filter=U` с `LC_ALL=C` (приём из Automaker). При конфликте merge откатывается, создаётся run `resolve_conflict` в отдельном worktree с инструкцией и списком файлов. Если и он не справился, задача получает статус `failed` с пометкой «нужен ручной merge».
- Удаление worktree: `git worktree remove --force` + `prune`; удалять только пути внутри `~/.studio/worktrees` после `realpath` (guard как у Superset).
- В основную ветку пользователя Studio **ничего не мержит сам**. Результат — интеграционная ветка; кнопка «Создать PR» (через `gh`, если установлен) или «Показать команды для merge».

### 3.5 Движок (`packages/engine`)

1. **Mapper.** `Run(kind=map)`: агент читает репозиторий и пишет `RepoMapJson`. Кэш по `commitSha`; кнопка «Обновить карту».
2. **Planner.** `Run(kind=plan)`: на вход запрос + RepoMap + выдержки файлов по выбору агента. Агент пишет 2 `PlanVariantJson`. Валидации сверх zod:
   - ключи задач уникальны, `dependsOn` ссылается на существующие задачи, граф ацикличен;
   - `componentId` существуют в RepoMap;
   - у каждой задачи минимум 1 критерий приёмки.
   При нарушении — follow-up с ошибками.
3. **Approval.** Пользователь меняет title, description, dependsOn, scope, acceptance и агента у задачи, может удалять и добавлять задачи. Перед утверждением та же валидация.
4. **Scheduler.** Топологическая сортировка; в работу идут задачи, у которых все зависимости в `done` **и уже влиты** в интеграционную ветку; лимит `maxConcurrency` (по умолчанию 3). Резолвер зависимостей можно портировать из Automaker (`libs/dependency-resolver/src/resolver.ts`, MIT, с атрибуцией). После `done` задача сразу вливается (п. 3.4), затем scheduler пересчитывает готовые задачи.
5. **Implement prompt** включает: описание задачи, scope.allow/deny, критерии приёмки, краткую выжимку RepoMap, список уже выполненных задач плана с их коммитами, правило «коммить в конце; не трогай файлы вне scope без необходимости, а если тронул — объясни в финальном сообщении».
6. **Verifier** после интеграции всех задач:
   - (a) `testCommand` проекта в интеграционном worktree;
   - (b) **scope-check без LLM**: файлы из diff каждой задачи сверяются с её glob-ами;
   - (c) `Run(kind=verify)`, только чтение: на вход план, общий diff, результаты (a) и (b); на выход `VerifyReportJson` с вердиктом по каждому критерию и `unplannedChanges`. Агент-верификатор по умолчанию **другой**, чем исполнявший большинство задач (настройка).
7. **Follow-up из ревью.** Инлайн-комментарии собираются в markdown (формат как у Vibe Kanban, `ReviewProvider.tsx`) и отправляются в новую сессию по задаче; затем повторная интеграция и верификация.

Все промпты лежат в `packages/prompts/*.md` с плейсхолдерами; хардкод промптов в коде запрещён.

### 3.6 UI (renderer)

Левая колонка: проекты → планы проекта. Основная область — вкладки плана:

| Вкладка | Содержимое |
|---|---|
| **Map** | Граф RepoMap (React Flow + dagre). При выбранном варианте затронутые компоненты подсвечены (add — зелёный, modify — жёлтый, remove — красный). Клик по компоненту показывает описание, пути и задачи, которые его трогают |
| **Plan** | Запрос; карточки вариантов рядом (подход, плюсы и минусы, риски, сложность); редактор задач выбранного варианта; граф задач (DAG); кнопка «Утвердить» |
| **Run** | Граф задач со статусами в реальном времени; по клику на задачу — лента нормализованных событий агента, запросы прав (approve / deny), кнопки cancel и retry, кнопка «Открыть терминал в worktree» |
| **Changes** | Список файлов + Monaco diff: по задаче и общий; инлайн-комментарии; «Отправить комментарии агенту» |
| **Verify** | Отчёт: тесты, scope-нарушения, таблица критериев (pass / fail / unclear + ссылки, клик открывает diff на строке), незапланированные изменения, общий вердикт |

Плюс: экран настроек (агенты: обнаружение и версии; провайдеры OpenCode; прокси; `maxConcurrency`; язык), экран проекта (`testCommand`, `setupCommand`, allowlist команд). Нативные уведомления ОС на события `awaiting_input`, `failed`, `plan ready`, `verify done`.

Дизайн: плотный, «инструментальный», тёмная и светлая темы, без маркетинговой эстетики. Ориентир по плотности — Linear.

---

## 4. Что взять из open-source (с лицензиями)

Копировать код можно **только** из MIT и Apache-2.0, сохраняя copyright-заголовок и добавляя запись в `THIRD_PARTY_NOTICES.md`. Из ELv2, AGPL и GPL — **только идеи**, код не открывать в редакторе при написании соответствующего модуля.

| Репо | Лицензия | Что берём | Как |
|---|---|---|---|
| Automaker | MIT | Резолвер зависимостей (Kahn, приоритеты, детект циклов) — `libs/dependency-resolver/src/resolver.ts`; детект конфликтов с `LC_ALL=C` — `apps/server/src/services/merge-service.ts`; атомарная запись JSON | Код, с атрибуцией |
| Emdash | Apache-2.0 | ACP spawn-описания для claude, codex, opencode — `packages/plugins/src/agents/impl/{claude,codex,opencode}`; pipeline создания worktree (inspect → resolve-base → add → verify) — `packages/core/src/runtimes/workspace-registry/` | Код/фрагменты, с атрибуцией + NOTICE |
| Vibe Kanban | Apache-2.0 | Модель `NormalizedEntryType` / `ActionType` (`crates/executors/src/logs/mod.rs`); формат review-markdown (`packages/web-core/src/shared/hooks/ReviewProvider.tsx`); хранение `before/after_head_commit` на каждый run | Переписать на TS по образцу |
| Sculptor | MIT | Идея: заменить встроенные вопросы и plan-approval агента на нативный UI; их post-mortem отказа от Docker-контейнеров в пользу worktree (`docs/history.md`) — поэтому контейнеров в v0.1 нет | Идеи |
| Superset | ELv2 | Идеи: diff от merge-base, guard `rm` внутри корня worktree, карта статусов `working/permission/review/failed` | Только идеи |
| Maestro | AGPL-3.0 | Идеи: watchdog по тишине, capability-флаги агента | Только идеи |
| cmux | GPL-3.0 | Идея: не трогать глобальный конфиг агента | Только идеи |
| Claude Squad | AGPL-3.0 | Идея: «пауза» = коммит + удаление worktree с сохранением ветки | Только идеи |

---

## 5. Майлстоуны

Каждый майлстоун заканчивается запускаемым приложением, зелёными тестами и коротким `docs/milestones/Mx.md` (что сделано, что отложено, как проверить руками). Не начинать следующий, пока текущий не проходит свой acceptance.

**M0. Каркас (1–2 дня).** Monorepo, Electron + React + tRPC, drizzle + первая миграция, i18n ru/en, Biome, vitest, Playwright smoke «приложение открылось». CI-скрипт `pnpm check` = typecheck + lint + test.
*Acceptance:* `pnpm dev` открывает окно; `pnpm check` зелёный.

**M1. Проекты, агенты, терминал.** Добавить проект (выбор папки, проверка git). Обнаружение трёх агентов и их версий. ACP-сессия: простой чат с агентом в корне проекта, поток нормализованных событий, запрос прав в UI. Терминал (xterm + node-pty) в папке проекта.
*Acceptance:* в UI можно спросить Claude Code «что делает этот репозиторий» и увидеть стрим; тот же диалог через OpenCode.

**M2. Worktree + одиночная задача.** Пакет `git`: создание и удаление worktree, ветки, diff от merge-base, Monaco diff. Ручная «задача»: текст → worktree → implement-run → коммит → diff.
*Acceptance:* задача «добавь README-раздел X» выполняется в изолированном worktree, diff виден, основная ветка не тронута.

**M3. Карта и планирование.** Mapper, RepoMap-граф, Planner с 2 вариантами, валидация и повторы, UI Plan с редактированием и утверждением, подсветка затронутых компонентов.
*Acceptance:* на `tolerance-dev/arena` для запроса «голосование и рейтинг» получаем 2 валидных варианта и утверждаем один.

**M4. Исполнение и интеграция.** Scheduler с зависимостями и параллелизмом, интеграционная ветка, merge по готовности, `resolve_conflict`, вкладка Run с графом статусов, уведомления, восстановление после рестарта (`interrupted`).
*Acceptance:* утверждённый план из M3 исполняется, минимум 2 задачи идут одновременно, интеграционная ветка собирается.

**M5. Верификация и ревью.** testCommand, scope-check, verify-run, вкладка Verify, инлайн-комментарии → follow-up → повторная интеграция и верификация. Кнопка «Создать PR».
*Acceptance:* полный демо-сценарий из §1.

**M6. Полировка для первых пользователей.** Онбординг (проверка установленных агентов, подсказки по установке), экран провайдеров OpenCode (GigaChat, YandexGPT, DeepSeek, Ollama), прокси, сборка `.dmg` и `.AppImage` (electron-builder), README на русском.

---

## 6. Правила работы для исполнителя

- Работай по майлстоунам строго по порядку. В начале майлстоуна напиши короткий план в `docs/milestones/Mx.md`, в конце — итог.
- Любое отклонение от спеки или неочевидный выбор фиксируй одной строкой в `docs/DECISIONS.md` (дата, решение, почему).
- Типы — только из `packages/schema` (zod → `z.infer`). Никаких `any` и `as unknown as` без комментария-обоснования.
- Каждый сервис в `engine` и `git` покрыт unit-тестами. Git-тесты работают на временных репозиториях, созданных в тесте.
- Агентов в тестах мокать фейковым ACP-агентом (`packages/agents/test/fake-agent.ts`), который проигрывает заданный сценарий событий. Реальные агенты — только в ручной проверке.
- Не добавляй зависимости без нужды. Каждую новую runtime-зависимость кратко обосновывай в DECISIONS.
- Никакой телеметрии, сетевых вызовов на свои сервера и аккаунтов.
- Секреты (API-ключи провайдеров) — в системном keychain через `safeStorage` Electron, не в SQLite открытым текстом.
- Не выходи за не-цели §1, даже если кажется, что «это 20 строк».
