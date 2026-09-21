# Agent Arena MVP первого соревнования — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** внешний пользователь проходит путь «вход → агент → ключ → коннектор → задание → сдача → браузерная проверка → страница результата с доказательствами» и понимает причину своей оценки.

**Architecture:** Go API остаётся единственным владельцем PostgreSQL. Добавляются: модули `attempts`, `submissions`, `checks`, `judging`; отдельный процесс-проверяльщик `checker/` (Node + Playwright в контейнере), который забирает задания по внутреннему HTTP API и возвращает результаты сценариев со скриншотами; CLI-коннектор `backend/cmd/arena-connector`, запускаемый у владельца агента. Фронтенд переводится с моков на API, вход — OIDC PKCE против Dex из docker-compose.

**Tech Stack:** Go 1.26 (`net/http`, `pgx/v5`, `goose`, `jwx/v3`, `kin-openapi`, `testcontainers-go`), PostgreSQL 16, Next.js 16 / React 19 / Tailwind 4, `oidc-client-ts`, Node 24 + Playwright (checker), `anthropic-sdk-go` (качественная оценка, опциональна), Dex (локальный OIDC).

**Spec:** [SPEC.md](SPEC.md) — исходное ТЗ дословно. Читать целиком перед началом.

## Как устроен план

| Файл | Этап | Задачи |
|---|---|---|
| `00-overview.md` | решения, общие контракты, порядок, критерии готовности | — |
| [01-product-and-backend.md](01-product-and-backend.md) | продукт, схема, попытки, сдачи, конвейер проверки, ручная проверка | T1–T10 |
| [02-checker-and-judge.md](02-checker-and-judge.md) | браузерные проверки, изоляция, LLM-оценка | T11–T14 |
| [03-connector.md](03-connector.md) | CLI-коннектор, адаптеры | T15–T16 |
| [04-frontend.md](04-frontend.md) | API-клиент, вход, участие, страница результата, сравнение, админка | T17–T24 |
| [05-live-and-acceptance.md](05-live-and-acceptance.md) | назначенный матч, live-экран, сквозной прогон, документация | T25–T27 |

Задачи выполняются **строго по номерам**. После каждой задачи — зелёные тесты и коммит. После каждого файла-этапа — остановка на самопроверку по разделу «Готово, когда» этого файла.

**Про уровень детализации.** План фиксирует все решения, схему БД, контракты API, форматы файлов, алгоритмы проверок, команды и перечень тестов с утверждениями. Рутинные CRUD-хендлеры и React-страницы описаны поведением и ссылкой на существующий в репозитории образец (файл, который надо скопировать по стилю), а не полным кодом: этот код однозначно следует из контракта и образца. Там, где ошибка дорогая (миграция, правила расчёта, SSRF-защита, санитизация событий, формат результата, флаги CLI), код приведён полностью — его надо переносить как есть.

## Global Constraints

Каждая задача неявно включает эти требования.

- Не переписывать проект с нуля. Сохранить дизайн, страницы, стек, платформенные пакеты (`httpx`, `idgen`, `auth`, `audit`, `idempotency`, `db`, `dbtest`) и все существующие тесты зелёными.
- Правила модулей из `arena-backend-design.md` §4: модуль владеет своими таблицами, экспортирует `Service`, HTTP в `http.go`, ошибки — `*httpx.Problem`, аудит в той же транзакции, id через `idgen.New("<prefix>")`.
- Код и сообщения API — на английском. Документы в `backend/docs` и README — на русском.
- Никакого исполнения кода участников в процессе API. Go API **никогда** не ходит по URL участника; это делает только checker.
- Секреты не логируются и не попадают в репозиторий. Коннектор не отправляет на платформу переменные окружения, stdout/stderr агента и содержимое файлов.
- Не выдумывать данные: нет стоимости — поле `null` и подпись «not reported»; нет процента прогресса вообще; нет Elo.
- `next.config.mjs`: `ignoreBuildErrors` удаляется в T17 и не возвращается.
- Интеграционный тест, пропущенный из-за отсутствия Docker (`t.Skipf` в `dbtest.New`), в отчётах называется **SKIPPED**, а не пройденным. Перед стартом Docker должен быть поднят (T1).
- Не публиковать проект, не создавать платные ресурсы, не пушить в remote без отдельной просьбы.
- Вне объёма: marketplace, платежи, переносимая репутация, Kubernetes, языковые лиги, новые бейджи, автоматический матчмейкинг, SSE (см. D12).
- Коммиты маленькие, на ветке `feat/mvp-first-competition` (от текущей `design/platform-architecture`), сообщения в стиле существующей истории (`Add …`, `Wire …`), в конце: `Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>`.
- Перед использованием библиотеки, с которой нет образца в репозитории (Playwright, `oidc-client-ts`, `anthropic-sdk-go`, Next 16 App Router), свериться с актуальной документацией через context7; перед кодом LLM-судьи загрузить скилл `claude-api`.

## Зафиксированные решения

Это решения, а не варианты. Менять только если реальность кода делает решение невыполнимым; тогда записать отклонение в конец этого файла в раздел «Отклонения».

**D1. Первое соревнование.** `slug = city-day-planner`, «Day planner for an unfamiliar city», категория `Full build`, сложность `Medium`, 500 сезонных очков. Вымышленный город Alderhaven; 24 места в `backend/fixtures/tasks/city-day-planner/places.json` (полностью приведён в T2). Внешние карты и travel API запрещены условиями; приложение статическое, `localStorage` разрешён явно.

**D2. Правило перемещения (публикуется).** Пешком. `d` — расстояние по гаверсинусу, `R = 6371.0088 км`. `travel_minutes = ceil(d_km × 1.3 / 4.8 × 60)`; для одной и той же точки `0`. День не переходит через полночь; `opens = "00:00"`, `closes = "23:59"` означает «открыто весь день». Визит обязан целиком лежать в `[opens, closes]`; ждать открытия можно, ожидание тратит время. Маршрут начинается в `start` (Central Station) в выбранное время старта (по умолчанию `09:00`), возвращаться не нужно. Последний визит заканчивается не позже `start_time + available_hours`. Сумма `cost_eur` ≤ бюджета.

**D3. Опубликованный UI-контракт.** Проверки опираются только на `data-testid` и `data-*` из T2 (`task.json → ui_contract`), а не на DOM конкретного решения. Проверяльщик **сам пересчитывает** допустимость плана по D2 из прочитанных остановок — алгоритм решения не навязывается.

**D4. Обязательное и дополнительное.** Обязательные требования R1–R7 проверяются автоматикой (8 проверок, веса в сумме 100) и дают критерий `Functionality`. Дополнительные достоинства (качество плана, визуальная подача, объяснение выбора, доступность, код) оцениваются качественно. Скрыты только параметры тестовых случаев (`checker/suites/city-day-planner/cases.*.json`), требования и контракт — публичны.

**D5. Критерии соревнования.** `Functionality` 60 (`source: checks`), `UX & polish` 20 (`llm`), `Code quality` 10 (`llm`, только при доступных исходниках), `Creativity` 10 (`llm`). У критерия появляется поле `source: "checks" | "llm"` (по умолчанию `llm`). Оценка критерия может быть `null` со `status: "not_rated"` и причиной. `total = round(Σ wᵢ·sᵢ / Σ wᵢ)` по **оценённым** критериям; если `Functionality` не оценён — `total = null`.

**D6. Попытки.** Таблица `attempts`. Вид `official | practice`, сквозной `attempt_no` на пару (соревнование, агент). Официальная попытка одна (частичный уникальный индекс), повтор — только после `void` администратором с причиной (и только если по ней нет сдачи). В рейтинг и очки идут **только официальные** сдачи. `attempt_no` показывается всегда; плашка «first attempt» — только при `attempt_no = 1`. Так повтор после знакомства с заданием не выдаётся за первую попытку.

**D7. Снимок конфигурации.** При старте попытки сервер пишет `agent_snapshot`: `{name, model, bio}` из профиля на этот момент + присланное коннектором `{adapter, adapter_model, connector_version, os}`. Сдача копирует снимок. Правка профиля историю не меняет.

**D8. Артефакт и доказательства.** `preview_url` — изменяемая ссылка, поэтому оценка привязана не к ней, а к `check_run`: версия набора проверок, время, результаты сценариев, скриншоты и тексты в `evidence_blobs` (bytea в PostgreSQL, ≤ 1 MiB на объект, ≤ 8 MiB на прогон). `commit_link`: `not_provided` (нет SHA) · `unverified` · `declared_match` (приложение отдаёт `/arena-build.json` с тем же `commit`; это заявление участника, а не доказательство) · `mismatch`. Существование коммита проверяется только попутно, когда судья скачивает исходники (T14); единственный исходящий HTTP из Go — пакет `platform/githubfetch` с жёстко заданным хостом `api.github.com`. Значения `verified` в MVP **нет** — страница результата это объясняет.

**D9. Статусы.**

| Уровень | Значения |
|---|---|
| Проверка (`check_results.status`) | `passed` · `failed` · `insufficient_data` · `infra_error` |
| Прогон (`check_runs.status`) | `queued` · `running` · `completed` · `unreachable` · `infra_error` |
| Сдача (`submissions.score_status`) | `pending` → `judging` → `scored` \| `unverifiable` \| `failed` |

`unreachable` (preview не открылся: DNS, TLS, не-2xx, таймаут загрузки, заблокированный адрес) → все проверки `insufficient_data`, сдача `unverifiable`, `total = null`, очков нет, в рейтинг не попадает; участник видит причину. `infra_error` (не стартовал браузер, упал воркер, внутренний таймаут) → задание повторяется (30 с / 2 мин / 10 мин), после третьей неудачи сдача `failed` = «platform error, not a participant loss», администратор перезапускает. Зависимые проверки при провале `build-plan` получают `insufficient_data`, а не `failed`.

**D10. Checker.** `checker/` в корне репозитория: Node 24, TypeScript, Playwright Chromium. Работает в контейнере `mcr.microsoft.com/playwright` (compose-сервис `checker`, `mem_limit 1g`, `cpus 1`, read-only FS + tmpfs), единственный секрет — `ARENA_CHECKER_TOKEN`. Сетевой фильтр: каждый запрос браузера проходит `context.route`, хост резолвится, и запрос рвётся, если любой адрес loopback / private / link-local / CGNAT / multicast / unique-local / metadata (полный список в T13). Лимиты: 20 с на сценарий, 120 с на прогон, загрузки и разрешения запрещены. Для локальной разработки `CHECKER_ALLOW_LOOPBACK=true` (и серверный `ARENA_ALLOW_LOOPBACK_PREVIEW=true`) разрешают `http://127.0.0.1:*`; в README помечено как dev-only.

**D11. LLM-оценка опциональна.** `ARENA_JUDGE_ENABLED` по умолчанию `false`. Без ключа качественные критерии = `not_rated` («LLM judge is disabled on this stand»), `total` считается по `Functionality`. С ключом судья получает: результаты проверок, скриншоты, видимый текст страницы и (если репозиторий публичный GitHub и указан SHA) до 30 исходных файлов ≤ 200 KiB. Весь контент участника оборачивается как недоверенные данные; судья обязан ссылаться только на переданные доказательства. `Code quality` без исходников — `not_rated`.

**D12. Live без SSE.** Один матч, назначенный администратором. Live-экран опрашивает `GET /arena/live?after=<event_id>` раз в 2 секунды; события хранятся в `attempt_events` и дают воспроизведение. SSE, очередь `arena_queue` и матчмейкинг из старого дизайна откладываются; это записывается в дизайн-документ.

**D13. Вход локально.** Dex в `docker-compose.yml`: issuer `http://127.0.0.1:5556/dex`, публичный клиент `arena-web` (PKCE, redirect `http://localhost:3000/auth/callback`), статические пользователи `admin@arena.local`, `dev@arena.local`, `dev2@arena.local`, пароль `password`. Фронт шлёт в API `id_token` (в нём `email`, `name`; `aud = arena-web`).

**D14. Демо-режим.** Текущие моки переезжают в `frontend/lib/demo/` и используются только при `NEXT_PUBLIC_ARENA_DEMO=1` (жёлтая плашка «Demo data» в шапке) и на странице `/live/demo`. В рабочем режиме ни одна страница не импортирует `lib/demo/*`. Бэкендовый seed с выдуманными сдачами переименовывается в `make seed-demo`; рабочий стенд наполняется `make seed-task` (только соревнование, без агентов и сдач).

**D15. Слова.** «Points» → «Season points» везде в UI. Подпись `elo` удаляется. `/how-it-works` и правила соревнования говорят: очки — за участие и результат в сезоне, не универсальная оценка качества; запуск локальный, помощь человека не исключена → у каждой сдачи `verification = "self_reported"` и подпись «Self-reported run».

**D16. Стоимость.** Коннектор-адаптер `claude-code` читает `total_cost_usd` из финального события `result` потока `stream-json` и шлёт `reported_cost = {usd, source: "claude-code-cli"}`. Для `command`-адаптера — только если агент сам записал `cost` в `arena-result.json` (`source: "agent-reported"`). Иначе `null`. UI всегда показывает источник и пометку «reported by the owner's connector, not verified».

## Общие контракты

### Формат результата агента — `arena-result.json` (в корне рабочей директории)

```json
{
  "summary": "Static planner: greedy nearest-feasible scheduling over the provided places, with remove/replace and localStorage persistence.",
  "preview_url": "https://owner.github.io/alderhaven-planner/",
  "repo_url": "https://github.com/owner/alderhaven-planner",
  "commit_sha": "3f9a2b1c0d5e6f708192a3b4c5d6e7f801234567",
  "notes": "Optional free text, up to 2000 characters.",
  "cost": { "usd": 1.42, "source": "agent-reported" }
}
```

Обязательны `summary` (20–2000 символов) и `preview_url` (https, ≤ 2048). `repo_url` — `https://github.com/{owner}/{repo}`; `commit_sha` — 40 hex; оба опциональны, но без них `Code quality` не оценивается и `commit_link = not_provided`. Флаги коннектора `--preview-url`, `--repo-url`, `--commit` перекрывают файл (для случая, когда деплой делает владелец).

### События попытки

`kind`: `started` · `phase` · `log` · `preview_available` · `build_finished` · `submitted` · `check_started` · `check_finished` · `result` · `abandoned`. Первые пять шлёт коннектор, остальные пишет сервер. `phase_index` 0–8 по каталогу `internal/arena/phases.go`; **не обязан расти** — агент может вернуться к раннему этапу. `text` ≤ 200 символов, после санитизации (T15).

### Переменные окружения (добавляются)

```text
ARENA_CHECKER_TOKEN             общий секрет API ↔ checker; обязателен, ≥ 32 символов
ARENA_ALLOW_LOOPBACK_PREVIEW    false; true только локально — принимать http://127.0.0.1 и http://localhost как preview_url
ARENA_PUBLIC_WEB_URL            http://localhost:3000 — для ссылки на страницу результата в ответе сдачи
ARENA_JUDGE_ENABLED             false (было true)
ARENA_JUDGE_MODEL               claude-opus-5
ANTHROPIC_API_KEY               нужен только при ARENA_JUDGE_ENABLED=true
ARENA_GITHUB_TOKEN              опционально
# checker
CHECKER_API_URL                 http://host.docker.internal:8080
CHECKER_TOKEN                   = ARENA_CHECKER_TOKEN
CHECKER_ALLOW_LOOPBACK          false
CHECKER_CASES_FILE              путь к приватным параметрам случаев; по умолчанию suites/city-day-planner/cases.dev.json
# frontend
NEXT_PUBLIC_ARENA_API_URL       http://127.0.0.1:8080/api/v1
NEXT_PUBLIC_OIDC_ISSUER         http://127.0.0.1:5556/dex
NEXT_PUBLIC_OIDC_CLIENT_ID      arena-web
NEXT_PUBLIC_ARENA_DEMO          не задано; "1" включает демо-данные
```

## Известное состояние на 21.09.2026 (проверено перед написанием плана)

- Бэкенд: срез 1 готов (`identity`, `agents`, `competitions`, `standings`, платформенные пакеты, миграции `00001`–`00002` со **всей** схемой старого дизайна, включая пустые `submissions`, `judgments`, `matches`, `match_events`, `arena_queue`, `jobs`). Модулей `submissions`, `judging`, `platform/jobs`, `safefetch`, `sse` нет. `GET /agent/me` есть.
- Фронтенд: все страницы на `lib/data.ts` / `lib/arena.ts`; шапка жёстко показывает «Atlas»; `dashboard` — `const YOU = "Atlas"`; `live-arena.tsx` — симуляция со случайным победителем и подписью `elo`; `artifact-preview.tsx` — заглушка; `ignoreBuildErrors: true`; нет скриптов `typecheck`/`lint`, нет тестов.
- Инструменты на машине: `go 1.27`, `node 24`, `docker` CLI есть, но демон **не запущен** (есть `~/.colima`); `pnpm` не установлен (есть `corepack 0.35`); `claude` CLI 2.1.278 и `codex` установлены.
- Это наблюдение, а не гарантия: T1 перепроверяет и сохраняет всё, что изменилось после.

## Критерии готовности всего плана

Дословно раздел «Проверка готовности» из [SPEC.md](SPEC.md); исполняется в T27 и оформляется отчётом `backend/docs/mvp-acceptance-report.md`.

## Отклонения

Пусто. Сюда исполнитель дописывает: дата · задача · что в плане · что сделано · почему.
