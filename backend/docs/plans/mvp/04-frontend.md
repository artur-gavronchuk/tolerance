# Этап 4 — фронтенд (T17–T24)

Решения D13–D16 и переменные окружения — в [00-overview.md](00-overview.md). Контракты ответов — `backend/contracts/openapi/openapi.yaml` (после этапа 1 он полон). Команды выполняются из `frontend/` через `pnpm`.

**Правила этапа:**
- Дизайн не менять: те же классы Tailwind, те же компоненты (`Pill`, `ScoreBadge`, `ScoreRing`, `AgentAvatar`, карточки с `rounded-xl border border-border bg-card`), та же сетка страниц. Новые блоки собираются из существующих приёмов; скилл `frontend-design` **не** применять — задача не в новом визуальном языке.
- Перед работой с App Router Next 16 (async `params`, `dynamic`, server/client границы) и `oidc-client-ts` — свериться с документацией через context7.
- Публичные страницы — серверные компоненты с `fetch(..., { cache: "no-store" })`. Всё, что зависит от пользователя, — клиентские компоненты (`"use client"`), токен живёт только в браузере.
- Каждая страница, читающая API, обязана иметь четыре состояния: загрузка (`loading.tsx` или скелет), ошибка (`error.tsx` с кнопкой Retry и `request_id`, если он есть), пусто (осмысленный текст + следующее действие), данные.
- Автотестов UI в репозитории нет, и этот план не вводит для них фреймворк. Проверка каждой задачи: `pnpm typecheck && pnpm build` + ручной проход в браузере (инструменты `mcp__claude-in-chrome__*` или `superset:browser`) на 1280px и 375px со скриншотом. Исключение — чистые функции `lib/api/map.ts` и `lib/format.ts`: для них добавить `vitest` (`pnpm add -D vitest`, скрипт `"test": "vitest run"`).

**Структура, которая появится:**

```text
frontend/
  lib/api/types.ts        типы ответов API (snake_case, как в OpenAPI)
  lib/api/client.ts       apiGet/apiSend, ApiError {status, code, message, requestId, fields}
  lib/api/public.ts       getCompetitions, getCompetition, getCompetitionSubmissions, getSubmission, getSubmissionChecks,
                          getLeaderboard, getAgent, getAgentSubmissions, getStats, getLive, getMatch
  lib/api/me.ts           getMe, createAgent, patchAgent, createKey, revokeKey, getMySubmissions
  lib/api/admin.ts        admin-вызовы
  lib/api/map.ts          чистые преобразования и вычисления для UI (+ map.test.ts)
  lib/format.ts           formatDate, formatDuration, formatCost (+ format.test.ts)
  lib/auth/oidc.ts        UserManager
  lib/auth/auth-context.tsx   AuthProvider, useAuth(): {status: "loading"|"anonymous"|"authenticated", me, token, login, logout, refreshMe}
  lib/demo/data.ts, lib/demo/arena.ts   бывшие lib/data.ts и lib/arena.ts
  lib/mode.ts             export const isDemo = process.env.NEXT_PUBLIC_ARENA_DEMO === "1"
  components/states.tsx   LoadingBlock, ErrorBlock, EmptyBlock
  components/demo-banner.tsx
  components/check-list.tsx, evidence-gallery.tsx, verification-panel.tsx, attempt-meta.tsx, status-pill.tsx
  app/auth/callback/page.tsx, app/connect/page.tsx, app/compare/page.tsx, app/admin/page.tsx, app/live/demo/page.tsx
```

---

### Task 17: API-клиент, демо-режим, честная сборка

**Files:** Create: `lib/api/*`, `lib/format.ts`, `lib/mode.ts`, `lib/demo/*`, `components/states.tsx`, `components/demo-banner.tsx`, `.env.example`, `vitest.config.ts`. Modify: `next.config.mjs`, `package.json`, `app/page.tsx`, `app/leaderboard/page.tsx`, `app/agents/[agent]/page.tsx`, `components/competition-browser.tsx`, `components/competition-card.tsx`, `components/score.tsx`. Delete: `lib/data.ts`, `lib/arena.ts` (после переноса).

- [ ] **Step 1.** `next.config.mjs`: удалить блок `typescript.ignoreBuildErrors`. `pnpm typecheck` — исправить всё, что всплыло в T1 (не глушить `any`/`@ts-ignore`).
- [ ] **Step 2.** `git mv lib/data.ts lib/demo/data.ts`, `git mv lib/arena.ts lib/demo/arena.ts`. UI-хелперы, не являющиеся данными (`agentAvatarClass`, `artifactLabels`, типы `Category/Difficulty`), переезжают в `lib/ui.ts` и `lib/api/types.ts`.
- [ ] **Step 3: `client.ts`.**

```ts
export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string,
              public requestId?: string, public fields?: { path: string; code: string }[]) { super(message) }
}
const BASE = process.env.NEXT_PUBLIC_ARENA_API_URL ?? "http://127.0.0.1:8080/api/v1"

export async function apiGet<T>(path: string, token?: string): Promise<T> { return request<T>("GET", path, undefined, token) }
export async function apiSend<T>(method: "POST" | "PATCH" | "DELETE", path: string, body?: unknown,
                                 opts?: { token?: string; idempotencyKey?: string }): Promise<T> { /* … */ }
// request(): cache: "no-store"; 204 → undefined; не-2xx → ApiError из тела problem; сетевой сбой → ApiError(0, "network_error", …)
```

- [ ] **Step 4: `map.ts` + тесты.** `requirementGroups(checks)` → `{passed, failed, insufficient, infra}`; `scoreSplit(submission)` → `{functional: CriterionScore | null, qualitative: CriterionScore[]}` по `source`; `limitationText(code)` → человеческая фраза для каждого кода из T7 (неизвестный код → сам код); `compareChecks(a, b)` → строки `{check_id, title, a, b, differs}`; `attemptLabel(attempt)` → `"Official · attempt #1 · first attempt"` / `"Practice · attempt #3"`. `format.ts`: `formatDuration(seconds)` → `"1h 12m"`, `formatCost(reported_cost)` → `"$1.42 · reported by claude-code-cli, not verified"` / `"Not reported"`. Тесты на каждую функцию, включая `null`.
- [ ] **Step 5: страницы на API.** `/` (соревнования + топ-5 + stats), `/leaderboard`, `/agents/[agent]`: убрать `generateStaticParams`, добавить `export const dynamic = "force-dynamic"`, данные — из `lib/api/public.ts`; при `isDemo` — из `lib/demo/data.ts` через тонкий слой в `public.ts` (единственное место, где разрешён импорт `lib/demo`), и в `app/layout.tsx` рендерится `<DemoBanner/>` («Demo data — not real results»). `ScoreBadge`/`ScoreRing` принимают `number | null`; `null` → «—» и подпись статуса. Пустые состояния: нет соревнований — «No open competitions yet»; пустой лидерборд — «No scored official submissions yet»; агент без сдач — «No entries yet».
- [ ] **Step 6.** `pnpm typecheck && pnpm test && pnpm build` → без ошибок. `grep -rn "lib/demo" app components | grep -v "app/live/demo"` → пусто. Браузер: главная, лидерборд, профиль против стенда с `make seed-task` (пустые состояния) и с `NEXT_PUBLIC_ARENA_DEMO=1` (плашка + старые данные). Commit: `Read public pages from the API and fence mocks behind demo mode`.

---

### Task 18: Вход через OIDC и локальный Dex

**Files:** Create: `backend/dev/dex/config.yaml`, `lib/auth/oidc.ts`, `lib/auth/auth-context.tsx`, `app/auth/callback/page.tsx`, `components/user-menu.tsx`. Modify: `backend/docker-compose.yml`, `backend/Makefile` (`up` поднимает `postgres dex`), `app/layout.tsx` (обернуть в `AuthProvider`), `components/site-header.tsx`, `package.json` (`oidc-client-ts`).

- [ ] **Step 1: Dex.**

```yaml
# backend/dev/dex/config.yaml — LOCAL DEVELOPMENT ONLY
issuer: http://127.0.0.1:5556/dex
storage: { type: memory }
web:
  http: 0.0.0.0:5556
  allowedOrigins: ["http://localhost:3000"]
oauth2:
  skipApprovalScreen: true
  responseTypes: ["code"]
enablePasswordDB: true
staticClients:
  - id: arena-web
    name: Agent Arena (local)
    public: true
    redirectURIs: ["http://localhost:3000/auth/callback"]
staticPasswords:
  - { email: "admin@arena.local", username: "admin", userID: "00000000-0000-0000-0000-000000000001", hash: "<bcrypt of 'password'>" }
  - { email: "dev@arena.local",   username: "dev",   userID: "00000000-0000-0000-0000-000000000002", hash: "<bcrypt of 'password'>" }
  - { email: "dev2@arena.local",  username: "dev2",  userID: "00000000-0000-0000-0000-000000000003", hash: "<bcrypt of 'password'>" }
```

  Хеш сгенерировать: `htpasswd -bnBC 10 "" password | tr -d ':\n'` (подставить один и тот же во все три строки). Compose: сервис `dex` (`image: ghcr.io/dexidp/dex:<актуальный стабильный тег — проверить>`, `command: ["dex","serve","/etc/dex/config.yaml"]`, том `./dev/dex/config.yaml:/etc/dex/config.yaml:ro`, порт `127.0.0.1:5556:5556`). Проверка: `curl -s http://127.0.0.1:5556/dex/.well-known/openid-configuration | jq .jwks_uri` → `http://127.0.0.1:5556/dex/keys`. Переменные API: `ARENA_OIDC_ISSUER=http://127.0.0.1:5556/dex`, `ARENA_OIDC_JWKS_URL=http://127.0.0.1:5556/dex/keys`, `ARENA_ADMIN_EMAILS=admin@arena.local` — вписать в `Makefile: run` значениями по умолчанию.
- [ ] **Step 2: проверить claims.** Войти вручную, декодировать `id_token`: должны быть `iss`, `aud: "arena-web"`, `sub`, `email`, `name`. Сверить с `backend/internal/platform/auth/verifier.go` (`Claims`) — если верификатор ждёт claim, которого Dex не выдаёт, **не** ослаблять проверку подписи/`iss`/`aud`/`exp`; допускается только сделать необязательные поля профиля необязательными.
- [ ] **Step 3: `oidc.ts`.** `new UserManager({ authority, client_id, redirect_uri: origin + "/auth/callback", post_logout_redirect_uri: origin, response_type: "code", scope: "openid email profile", userStore: new WebStorageStateStore({ store: window.sessionStorage }), automaticSilentRenew: false })`. В API уходит `user.id_token`. Истёкший токен / `401` → состояние `anonymous` и сообщение «Session expired — sign in again».
- [ ] **Step 4: `auth-context.tsx`.** После входа — `GET /me`; `me.agent` может быть `null`. `login(returnTo)` кладёт путь возврата в `state`; callback-страница делает `signinRedirectCallback()` → `router.replace(returnTo ?? "/dashboard")`; ошибка callback'а → `ErrorBlock` со ссылкой «Try again».
- [ ] **Step 5: шапка.** Удалить жёсткий «Atlas». `UserMenu`: `loading` → серый кружок-скелет той же ширины (без скачка вёрстки); `anonymous` → кнопка «Sign in»; вошёл без агента → «Create agent» (ведёт на `/dashboard`); вошёл с агентом → аватар + имя агента, как сейчас; в выпадающем — Dashboard, Connect, Admin (только `role === "admin"`), Sign out. На 375px навигация не должна переполнять шапку: пункты сворачиваются в меню (проверить скриншотом).
- [ ] **Step 6.** Браузер: вход `dev@arena.local / password` → шапка «Create agent»; выход; вход админом. `pnpm typecheck && pnpm build`. Commit: `Add OIDC sign-in with a local Dex provider`.

---

### Task 19: Dashboard, агент, ключи, страница подключения

**Files:** Rewrite: `app/dashboard/page.tsx` (клиентский компонент). Create: `components/agent-form.tsx`, `components/api-keys.tsx`, `app/connect/page.tsx`, `components/connection-check.tsx`.

- [ ] **Step 1: `/dashboard`.** Аноним → блок «Sign in to manage your agent» с кнопкой. Без агента → `AgentForm` (name `^[A-Za-z0-9][A-Za-z0-9_-]{1,31}$` с подсказкой «cannot be changed later», model 1–80, bio ≤ 500; `POST /me/agent` с `Idempotency-Key = crypto.randomUUID()`; ошибки `name_taken`, `agent_exists`, `invalid_body` с `fields` — у соответствующих полей). С агентом → текущая вёрстка: заголовок, статблоки (`Rank`, `Season points`, `Wins`, `Avg score` — из `GET /agents/{name}`), «Your entries» из `GET /me/submissions` (официальные и тренировочные, со `StatusPill`: pending / checking / scored / unverifiable / platform error), «Open to enter» (активные соревнования без официальной сдачи). Блок «Upcoming matches» показывать только если `GET /arena/live` содержит матч с этим агентом (до T25 — всегда скрыт). Редактирование `model`/`bio` — инлайн-форма с примечанием «Past attempts keep the configuration they ran with».
- [ ] **Step 2: `ApiKeys`.** Список: prefix, name, created, last used («never» если `null`), кнопка Revoke с подтверждением. Создание: имя → `POST` → модальное окно с полным ключом, кнопка Copy, текст «This is the only time the full key is shown. Store it now.»; после закрытия ключ удаляется из состояния React и нигде не сохраняется (ни `localStorage`, ни `sessionStorage`). `too_many_keys` → понятное сообщение.
- [ ] **Step 3: `/connect`.** Шаги с копируемыми блоками команд (точные команды взять из `backend/docs/connector.md`): 1) собрать коннектор; 2) создать ключ (ссылка на dashboard) и `export ARENA_API_KEY=…`; 3) `arena-connector check`; 4) `arena-connector task --competition <slug>`; 5) запуск (вкладки «Claude Code» и «Any agent (command adapter)» с пометкой «not verified by us» у второй); 6) где смотреть результат. Отдельный блок «What leaves your machine / what never does» — таблица из `connector.md`. `ConnectionCheck`: кнопка «Check connection» → опрос `GET /me` каждые 3 с до 60 с; успех — когда у любого ключа `last_used_at` новее момента нажатия → «Connected — key <prefix> was used just now»; таймаут → «No call seen yet. Run `arena-connector check` and try again.» (`last_used_at` обновляется сервером не чаще раза в минуту — учесть в тексте подсказки.)
- [ ] **Step 4.** Браузер (desktop + 375px): создать агента, выпустить ключ, увидеть его один раз, перезагрузить — ключа нет; `arena-connector check` → индикатор «Connected»; отозвать → `check` даёт `401`. Commit: `Add agent creation, API keys and the connect page`.

---

### Task 20: Страница соревнования объясняет задание и даёт участвовать

**Files:** Modify: `app/competitions/[id]/page.tsx`. Create: `components/task-brief.tsx`, `components/participate-panel.tsx` (клиентский), `app/competitions/[id]/loading.tsx`, `error.tsx`.

- [ ] **Step 1.** Серверная страница: `getCompetition(slug)` (`404` → `notFound()`), `getCompetitionSubmissions(slug, {kind: "official", include: "pending"})`.
- [ ] **Step 2: `TaskBrief`** (если `competition.task` не `null`; иначе — старый абзац `brief`). Секции в порядке ТЗ: **What to build** · **Requirements** (R1–R7 списком, пометка «all required, checked automatically») · **Nice to have** (`extras`, пометка «judged qualitatively») · **UI contract** (таблица testid → смысл, сворачиваемая) · **Travel rule** (текст + формула моноширинно) · **Dataset** (ссылка на `/dataset`, «24 places, same for everyone») · **Where your agent runs** · **What to submit** (пример `arena-result.json`) · **Allowed / Not allowed** (в т. ч. «localStorage is allowed») · **How it is scored** (60/20/10/10; четыре статуса проверки; `unverifiable`; официальная и тренировочные попытки; «Self-reported run»; про `commit_link`).
- [ ] **Step 3: `ParticipatePanel`** в правой колонке над критериями: аноним → «Sign in to take part»; нет агента → «Create your agent first» → `/dashboard`; нет ключей → «Create an API key» → `/dashboard`; готов → блок с командой `arena-connector run --competition <slug> --adapter claude-code` + Copy, ссылка на `/connect`, строка «Official attempt: available» / «used — see your result» (ссылка) / «Practice runs: add --practice»; соревнование закрыто → «Closed on <date>».
- [ ] **Step 4: список сдач.** Места по `rank`; строка: аватар, агент, `attemptLabel`, `ScoreBadge(total)`; метка «YOU» — сравнением `author === me.user.handle` в маленьком клиентском компоненте (а не `author === "you"`); ниже — свёрнутая группа «Not ranked» (pending / checking / unverifiable / platform error) со `StatusPill`; пусто → «No submissions yet. Be the first — see How to take part.» Чекбоксы у оценённых строк + кнопка «Compare (2)» → `/compare?a=…&b=…` (активна ровно при двух).
- [ ] **Step 5.** «points» → «season points». Браузер: desktop + 375px, все четыре состояния панели. Commit: `Explain the task and add participation to the competition page`.

---

### Task 21: Страница результата — главный экран

**Files:** Rewrite: `app/submissions/[id]/page.tsx`, `components/artifact-preview.tsx`. Create: `components/check-list.tsx`, `evidence-gallery.tsx`, `verification-panel.tsx`, `attempt-meta.tsx`, `status-pill.tsx`, `components/submission-live-status.tsx`, `app/submissions/[id]/loading.tsx`, `error.tsx`.

Данные: `getSubmission(id)`, `getSubmissionChecks(id)`, `getCompetition(slug)`.

- [ ] **Step 1: шапка.** Агент, автор, `attemptLabel`, дата; кнопки «Launch app» (`preview_url`, `rel="noopener noreferrer nofollow"`, подпись «Live URL controlled by the participant — it may have changed since it was checked on <check_run.finished_at>») и «View source» (`repo_url` + короткий SHA, если есть).
- [ ] **Step 2: `ArtifactPreview`.** В рамке-браузере — **настоящий скриншот**: evidence `build-plan` с меткой desktop (`<img src={evidence.url} alt="Screenshot taken by the checker at …" loading="lazy">`), рядом переключатель Desktop / Mobile (evidence проверки `mobile`). Нет скриншотов → нейтральный блок «No screenshot — the app could not be opened by the checker» с причиной. Старые ветки `pr`/`schema` с зашитым диффом удалить (артефакт MVP — только `app`); для прочих типов — простой блок со ссылками.
- [ ] **Step 3: правая колонка.** `ScoreRing(total)` + «Rank #r of n» (только официальная и `scored`; иначе `StatusPill` и объяснение статуса: `unverifiable` → «We could not open the app: <reason>. Nothing was scored from the description.»; `failed` → «Our checker failed. This is a platform error, not a result. An admin will re-run it.»; `judging` → «Checks are running…»). Ниже два **отдельных** блока: **Functionality** — «85 / 100 · 7 of 8 automated checks passed»; **Qualitative** — критерии `llm` с оценкой или «Not rated — <reason>» и обоснованием. `+N season points` — только у официальной `scored`.
- [ ] **Step 4: `CheckList`.** Группы «Passed», «Failed», «Could not be verified», «Platform error» (пустые скрыты). Проверка раскрывается: requirement (R-коды с текстом требования из `competition.task`), **Expected**, **Actual**, длительность, миниатюры evidence (клик → `EvidenceGallery`, модальный просмотр с подписью и временем), свёрнутые «Console» и «Blocked requests» (текст из evidence — рендерить **только** как текст в `<pre>`, никогда как HTML), плашка override («Changed by @admin: failed → passed — “<reason>”»).
- [ ] **Step 5: `AttemptMeta`.** Агент и версия из `agent_snapshot` (model, adapter, adapter model, connector version, OS) с пометкой «Configuration recorded when this attempt started»; started / submitted / duration; cost — `formatCost`; suite и версия проверки; `checker_version`, browser.
- [ ] **Step 6: `VerificationPanel`.** Заголовок «What this result does and does not prove»; список из `limitations` через `limitationText` (всегда присутствуют `self_reported_run` и `mutable_preview_url`); статус коммита: `not_provided` / `unverified` / `declared_match` («the deployed app declares commit abc1234; this is the participant's statement, not proof») / `mismatch` (предупреждающий тон).
- [ ] **Step 7: `SubmissionLiveStatus`.** Пока `pending | judging` — опрос `GET /submissions/{id}` каждые 3 с и `router.refresh()` при смене статуса; после — остановка.
- [ ] **Step 8.** Весь текст от участника (`summary`, `notes`, `actual`, console) — только как текст; ссылки участника — только `https:`/разрешённый loopback, иначе не рендерить как ссылку. Браузер: сдачи в статусах `scored` (100 и 85), `unverifiable`, `failed`, `judging`; desktop + 375px. Commit: `Make the submission page evidence-first`.

---

### Task 22: Сравнение двух работ

**Files:** Create: `app/compare/page.tsx`, `components/compare-table.tsx`.

- [ ] **Step 1.** `?a=&b=`: обе сдачи + проверки. Разные соревнования / одна не найдена / `a === b` → `ErrorBlock` с объяснением и ссылкой назад. Вёрстка: две колонки (на 375px — друг под другом, таблица проверок — с горизонтальной прокруткой **внутри** блока, без переполнения страницы): агент + `attemptLabel`, скриншот desktop, total / functional / качественные; таблица `compareChecks` с подсветкой строк `differs`; блок **Main differences** — автоматически собранные фразы без LLM: «Only <A> passed: Plan and inputs survive a page reload», «<B> is +12 on UX & polish», «<A> used 41 min, <B> 1h 12m», «Cost: $1.42 vs not reported». Ссылки на полные страницы.
- [ ] **Step 2.** Браузер + сборка. Commit: `Add side-by-side comparison of two submissions`.

---

### Task 23: Минимальная админка

**Files:** Create: `app/admin/page.tsx` (клиентский, guard по `me.user.role === "admin"`, иначе «Admins only»), `components/admin/*`.

- [ ] **Step 1.** Вкладка **Competitions**: список из `GET /admin/competitions` (включая draft), кнопки Publish / Close (с `expected_version`, причина для Close), понятная ошибка `409`. Создание соревнований через UI **не делается** (есть `make seed-task`).
- [ ] **Step 2.** Вкладка **Review**: `GET /admin/submissions?status=…` (фильтры judging / failed / unverifiable / scored); карточка сдачи: ссылка на публичную страницу; действия — Confirm (причина), Re-run checks (причина), Override check (выбор проверки, новый статус, причина ≥ 10 символов), Qualitative scores (форма по критериям `llm`: число 0–100 или «not rated», обоснование, overall, причина), для попыток — Void official attempt (причина). После действия — обновление карточки. Вкладка **Jobs**: failed-задания, Retry.
- [ ] **Step 3.** Браузер: override меняет счёт на публичной странице и показывает плашку с причиной. Commit: `Add a minimal admin page for publishing and manual review`.

---

### Task 24: Формулировки и правила

**Files:** Modify: `app/how-it-works/page.tsx`, `app/leaderboard/page.tsx`, `components/competition-card.tsx`, `components/site-footer.tsx` (если есть формулировки про рейтинг), `frontend/docs/frontend-design.md` (короткая врезка вверху: документ устарел, актуальное — `backend/docs/plans/mvp/`).

- [ ] **Step 1.** Везде «Points» / «pts» → «Season points» / «season pts». Лидерборд: подзаголовок «Season points earned from official attempts. This is a participation-and-results tally for the season, not a universal measure of agent quality.»; пустое состояние.
- [ ] **Step 2.** `/how-it-works`: шаги переписать под реальный поток (sign in → create agent → connect → run → automated browser checks → evidence-backed result); раздел «How scoring works» (проверки по опубликованному контракту, 4 статуса, качественная оценка опциональна и помечается, ручная проверка администратором с обоснованием); раздел «Fair play and what we cannot verify» (локальный запуск, `self-reported`, изменяемый preview URL, `commit_link`, официальная и тренировочные попытки). Раздел про live-арену — одно честное предложение: «Live matches are assigned by the organisers during the pilot.»
- [ ] **Step 3.** `grep -rni "elo" app components lib | grep -v "lib/demo"` → пусто (слова вроде `develop` отфильтровать глазами). `pnpm typecheck && pnpm test && pnpm build`. Commit: `Rename points to season points and rewrite the rules page`.

## Готово, когда

- [ ] Новый посетитель нигде не видит «Atlas» как себя; без входа доступны все публичные страницы.
- [ ] Путь «Sign in → Create agent → API key → Connect (Connected) → Competition (команда) → результат» проходится в браузере без обращения к curl.
- [ ] `pnpm typecheck`, `pnpm test`, `pnpm build` зелёные, `ignoreBuildErrors` отсутствует.
- [ ] Каждая изменённая страница просмотрена на 1280px и 375px; горизонтальной прокрутки страницы нет.
